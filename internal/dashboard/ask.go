package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// Ask — natural language over the board's OWN diagnostics.
//
// The command bar reads as a prompt, so operators type questions at it
// ("who is the top consumer of memory?"). It only accepted eight exact
// keywords and answered anything else with "Unknown command", which is a
// surface promising an affordance it does not have.
//
// The model is handed this machine's live doctor findings and can only select
// among them. The prompt goes through the configured Engine ABI connector,
// which may be local or remote; callers must use the returned route receipt
// rather than assuming locality.

// askTimeout bounds a single completion. Past this the operator is better
// served by a loud failure than an indefinite spinner.
const askTimeout = 45 * time.Second
const maxAskRequestBody = 32 << 10

type askRequest struct {
	Question string `json:"question"`
}

// askResponse carries SERVER-OWNED text. Findings are rendered from the
// DoctorReport, not from anything the model wrote, so no identifier the
// operator sees can have been paraphrased. Summary is the model's one free-text
// field and is dropped entirely if it contains a factual token absent from the
// grounding.
type askResponse struct {
	Summary  string   `json:"summary"`
	Findings []string `json:"findings"`
	Model    string   `json:"model"`
	// Dropped reports that a summary was withheld and why, so the surface can
	// say so instead of silently showing less.
	Dropped string          `json:"dropped,omitempty"`
	Receipt *engine.Receipt `json:"receipt,omitempty"`
}

// modelSelection is the only thing the model returns.
type modelSelection struct {
	Findings []int  `json:"findings"`
	Summary  string `json:"summary"`
}

func severityLabel(sev guard.DiagnosticSeverity) string {
	switch int(sev) {
	case 0:
		return "OK"
	case 1:
		return "INFO"
	case 2:
		return "WARNING"
	case 3:
		return "CRITICAL"
	}
	return "UNKNOWN"
}

// groundingFromDoctor renders the live diagnostic report as the only facts the
// model may use.
func groundingFromDoctor(rpt *guard.DoctorReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workstation health score: %d/100 (status: %s)\n", rpt.Score, rpt.Status)
	b.WriteString("Diagnostic findings:\n")
	for i, f := range rpt.Findings {
		fmt.Fprintf(&b, "[%d] [%s] %s: %s\n", i, severityLabel(f.Severity), f.Check, f.Message)
		if f.Detail != "" {
			fmt.Fprintf(&b, "    detail: %s\n", f.Detail)
		}
	}
	return b.String()
}

// The model SELECTS; the server RENDERS. Asking for prose and hoping it stays
// faithful is what produced "action.runner…" for a finding that says
// "actions.runner…". Indices cannot be misspelled.
const askSystemPrompt = `You are Sirsi Pantheon's local machine-diagnostics assistant.

Below is a numbered list of live diagnostic findings for this machine.

Reply with ONLY a JSON object, no other text:
{"findings": [<indices>], "summary": "<one short sentence>"}

- "findings": the indices of the findings that answer the question, most
  relevant first, at most 4. Empty if none of them answer it.
- "summary": one plain-language sentence framing the answer. Do NOT restate
  process names, paths, labels, sizes, or numbers in it — the findings you
  selected are shown to the operator verbatim, so repeating their details only
  risks getting them wrong. Say what it MEANS.

If nothing in the list answers the question, use an empty findings array and say
so in the summary.`

// apiAsk answers a natural-language question about this workstation.
// POST /api/ask  {"question": "..."}
func (s *Server) apiAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxAskRequestBody))
	if err != nil {
		var limitErr *http.MaxBytesError
		if errors.As(err, &limitErr) {
			writeError(w, "request body exceeds the 32 KiB limit", http.StatusRequestEntityTooLarge)
		} else {
			writeError(w, "could not read request body", http.StatusBadRequest)
		}
		return
	}
	if err := validateAskJSON(body); err != nil {
		writeError(w, "invalid request JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateAskObjectKeys(body, "question"); err != nil {
		writeError(w, "invalid request JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var req askRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, "invalid request JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, "question is required", http.StatusBadRequest)
		return
	}
	executor, ok := s.cfg.EngineSelection.(EnginePromptExecutor)
	if !ok {
		writeError(w, "canonical engine selection and receipt controller is not configured", http.StatusServiceUnavailable)
		return
	}
	var acceptedPolicy *engine.RoutePolicy
	if policyExecutor, ok := executor.(EnginePromptPolicyExecutor); ok {
		policy := policyExecutor.Policy()
		acceptedPolicy = &policy
	}

	report, err := guard.Doctor()
	if err != nil {
		writeError(w, fmt.Sprintf("cannot read workstation diagnostics: %v", err), http.StatusInternalServerError)
		return
	}

	grounding := groundingFromDoctor(report)
	var sel modelSelection
	var model string
	sel, model, receipt, err := askSelectedEngineWithPolicy(r.Context(), executor, req.Question, grounding, acceptedPolicy)
	if err != nil {
		// Fail loud. A degraded answer here would be indistinguishable from a
		// real one, which is the failure mode this whole surface exists to avoid.
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	if receipt == nil || receipt.Route == nil || receipt.IdentityDigest == "" || receipt.RequestSHA256 == "" {
		writeError(w, "selected engine returned no verifiable route receipt", http.StatusServiceUnavailable)
		return
	}
	resp := askResponse{Model: model, Receipt: receipt}

	// Findings are rendered from the report, never from model text. An index
	// out of range is dropped rather than guessed at.
	for _, i := range sel.Findings {
		if i < 0 || i >= len(report.Findings) {
			continue
		}
		f := report.Findings[i]
		line := fmt.Sprintf("[%s] %s — %s", severityLabel(f.Severity), f.Check, f.Message)
		resp.Findings = append(resp.Findings, line)
		if len(resp.Findings) == 4 {
			break
		}
	}

	// The summary is the model's only free text. Fail closed: if it names
	// anything factual that is not in the grounding verbatim, it is withheld
	// entirely rather than shown with a caveat. The findings above still answer
	// the question, and they are exact.
	if sum := strings.TrimSpace(sel.Summary); sum != "" {
		if bad := unverifiableTokens(sum, grounding); len(bad) > 0 {
			resp.Dropped = "summary withheld — it contained " + strings.Join(bad, ", ") +
				", which does not appear in this machine's diagnostics"
		} else {
			resp.Summary = sum
		}
	}

	if len(resp.Findings) == 0 && resp.Summary == "" {
		resp.Dropped = strings.TrimSpace(resp.Dropped + " · nothing in the current diagnostics answers that")
	}

	writeJSON(w, resp)
}

// validateAskJSON rejects duplicate object members and trailing JSON values
// before decoding the public prompt request. encoding/json otherwise accepts
// duplicate names and silently lets the last one win.
func validateAskJSON(body []byte) error {
	if !utf8.Valid(body) {
		return fmt.Errorf("request JSON is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := walkAskJSONValue(decoder); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func validateAskObjectKeys(body []byte, allowed ...string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return err
	}
	if object == nil {
		return fmt.Errorf("JSON value must be an object")
	}
	allowedKeys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedKeys[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowedKeys[key]; !ok {
			return fmt.Errorf("unexpected JSON key %q", key)
		}
	}
	return nil
}

func walkAskJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if _, exists := seen[name]; exists {
				return fmt.Errorf("duplicate object key %q", name)
			}
			seen[name] = struct{}{}
			if err := walkAskJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("request object did not terminate")
		}
	case '[':
		for decoder.More() {
			if err := walkAskJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("request array did not terminate")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
	return nil
}

// askSelectedEngine keeps the existing grounded-diagnostics contract while
// making the dashboard's selected engine authoritative. The model still only
// selects finding indices; Pantheon renders the findings from Doctor data.
func askSelectedEngine(ctx context.Context, executor EnginePromptExecutor, question, grounding string) (modelSelection, string, *engine.Receipt, error) {
	return askSelectedEngineWithPolicy(ctx, executor, question, grounding, nil)
}

func askSelectedEngineWithPolicy(ctx context.Context, executor EnginePromptExecutor, question, grounding string, policy *engine.RoutePolicy) (modelSelection, string, *engine.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	request := engine.PromptRequest{
		System:    askSystemPrompt + "\n\n--- LIVE DIAGNOSTIC REPORT ---\n" + grounding,
		Prompt:    question,
		MaxTokens: 400,
	}
	var completion engine.Completion
	var receipt engine.Receipt
	var err error
	if policy != nil {
		policyExecutor, ok := executor.(EnginePromptPolicyExecutor)
		if !ok {
			return modelSelection{}, "", nil, fmt.Errorf("selected engine cannot honor the accepted route policy")
		}
		completion, receipt, err = policyExecutor.CompletePromptWithPolicy(ctx, request, *policy)
	} else {
		completion, receipt, err = executor.CompletePrompt(ctx, request)
	}
	if err != nil {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine unavailable: %w", err)
	}
	if receipt.SessionID == "" || receipt.Identity.ModelID == "" || receipt.Route == nil {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine returned no verifiable route receipt")
	}
	if completion.Model != receipt.Identity.ModelID {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine returned model %q without matching receipt identity", completion.Model)
	}
	session := engine.Session{ID: receipt.SessionID, Identity: receipt.Identity, CreatedAt: receipt.StartedAt}
	if err := receipt.Validate(session); err != nil {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine returned an invalid route receipt: %w", err)
	}
	completionSum := sha256.Sum256([]byte(completion.Text))
	if receipt.CompletionSHA256 != hex.EncodeToString(completionSum[:]) {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine completion does not match its route receipt digest")
	}
	raw, err := cleanCompletion(completion.Text)
	if err != nil {
		return modelSelection{}, "", nil, err
	}
	if raw == "" {
		return modelSelection{}, "", nil, fmt.Errorf("selected engine returned an empty answer")
	}
	sel, err := parseSelection(raw)
	if err != nil {
		return modelSelection{}, "", nil, err
	}
	return sel, completion.Model, &receipt, nil
}

// parseSelection extracts the JSON object the model was asked for. Models
// wrap JSON in prose or fences often enough that locating the outermost braces
// is worth more than a strict decode that fails on a stray "Here you go:".
func parseSelection(raw string) (modelSelection, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return modelSelection{}, fmt.Errorf("selected engine did not return a finding selection")
	}
	object := []byte(raw[start : end+1])
	if err := validateAskJSON(object); err != nil {
		return modelSelection{}, fmt.Errorf("selected engine returned an ambiguous selection: %w", err)
	}
	if err := validateAskObjectKeys(object, "findings", "summary"); err != nil {
		return modelSelection{}, fmt.Errorf("selected engine returned an invalid selection schema: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(object, &fields); err != nil {
		return modelSelection{}, fmt.Errorf("selected engine returned an invalid selection object: %w", err)
	}
	findingsJSON, ok := fields["findings"]
	if !ok || bytes.Equal(bytes.TrimSpace(findingsJSON), []byte("null")) {
		return modelSelection{}, fmt.Errorf("selected engine selection requires findings as an array")
	}
	var sel modelSelection
	if err := json.Unmarshal(findingsJSON, &sel.Findings); err != nil {
		return modelSelection{}, fmt.Errorf("selected engine findings must be an integer array: %w", err)
	}
	summaryJSON, ok := fields["summary"]
	if !ok || bytes.Equal(bytes.TrimSpace(summaryJSON), []byte("null")) {
		return modelSelection{}, fmt.Errorf("selected engine selection requires summary as a string")
	}
	if err := json.Unmarshal(summaryJSON, &sel.Summary); err != nil {
		return modelSelection{}, fmt.Errorf("selected engine summary must be a string: %w", err)
	}
	if len(sel.Findings) > 4 {
		return modelSelection{}, fmt.Errorf("selected engine returned %d findings; at most 4 are allowed", len(sel.Findings))
	}
	seen := make(map[int]struct{}, len(sel.Findings))
	for _, index := range sel.Findings {
		if index < 0 {
			return modelSelection{}, fmt.Errorf("selected engine returned negative finding index %d", index)
		}
		if _, exists := seen[index]; exists {
			return modelSelection{}, fmt.Errorf("selected engine repeated finding index %d", index)
		}
		seen[index] = struct{}{}
	}
	return sel, nil
}
