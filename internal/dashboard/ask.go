package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
)

// Ask — natural language over the board's OWN diagnostics.
//
// The command bar reads as a prompt, so operators type questions at it
// ("who is the top consumer of memory?"). It only accepted eight exact
// keywords and answered anything else with "Unknown command", which is a
// surface promising an affordance it does not have.
//
// Two rules shape this, and they are the whole design:
//
//  1. NEVER answer from the model's own knowledge. The engine is handed this
//     machine's live doctor findings and told to answer from them or say it
//     cannot. A confident wrong answer about which process is eating RAM is
//     worse than no answer — it is the same failure as a green badge over a
//     dead service, just phrased in English.
//  2. NEVER leave the machine. The endpoint is hardcoded to loopback and the
//     port comes from the local SNE contract file. Pantheon has zero telemetry
//     (A11); routing operator questions about their own workstation to a cloud
//     model would break that quietly and completely.

// askTimeout bounds a single completion. A local 12B model answering a short
// grounded question is a few seconds; past this the operator is better served
// by a loud failure than a spinner.
const askTimeout = 45 * time.Second

// sneDefaultPort is the fallback when the contract file is missing. Prefer the
// file — a hardcoded port is exactly what made the fabric watchdog kill a
// healthy broker for hours after serving moved off 8765.
const sneDefaultPort = 8477

// snePort reads the port the local engine is actually serving on.
func snePort() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return sneDefaultPort
	}
	b, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-server.port"))
	if err != nil {
		return sneDefaultPort
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || p <= 0 || p > 65535 {
		return sneDefaultPort
	}
	return p
}

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
	Dropped string `json:"dropped,omitempty"`
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
const askSystemPrompt = `You are Horus, the local workstation monitor for Sirsi Pantheon.

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

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// apiAsk answers a natural-language question about this workstation.
// POST /api/ask  {"question": "..."}
func (s *Server) apiAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "bad request body", http.StatusBadRequest)
		return
	}
	req.Question = strings.TrimSpace(req.Question)
	if req.Question == "" {
		writeError(w, "question is required", http.StatusBadRequest)
		return
	}

	report, err := guard.Doctor()
	if err != nil {
		writeError(w, fmt.Sprintf("cannot read workstation diagnostics: %v", err), http.StatusInternalServerError)
		return
	}

	grounding := groundingFromDoctor(report)
	sel, model, err := askLocalEngine(r.Context(), req.Question, grounding)
	if err != nil {
		// Fail loud. A degraded answer here would be indistinguishable from a
		// real one, which is the failure mode this whole surface exists to avoid.
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	resp := askResponse{Model: model}

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

// askLocalEngine sends one grounded completion to the loopback SNE server.
func askLocalEngine(ctx context.Context, question, grounding string) (modelSelection, string, error) {
	port := snePort()
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", port)

	body, err := json.Marshal(chatRequest{
		Model: "local",
		Messages: []chatMessage{
			{Role: "system", Content: askSystemPrompt + "\n\n--- LIVE DIAGNOSTIC REPORT ---\n" + grounding},
			{Role: "user", Content: question},
		},
		MaxTokens:   400,
		Temperature: 0.2,
	})
	if err != nil {
		return modelSelection{}, "", fmt.Errorf("build request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return modelSelection{}, "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return modelSelection{}, "", fmt.Errorf("local engine not reachable on 127.0.0.1:%d — is ai.sirsi.gemma-broker running? (%v)", port, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return modelSelection{}, "", fmt.Errorf("local engine returned %s", resp.Status)
	}

	var out chatResponse
	if decErr := json.NewDecoder(resp.Body).Decode(&out); decErr != nil {
		return modelSelection{}, "", fmt.Errorf("local engine sent an unreadable response: %w", decErr)
	}
	if len(out.Choices) == 0 {
		return modelSelection{}, "", fmt.Errorf("local engine returned no choices")
	}
	raw, err := cleanCompletion(out.Choices[0].Message.Content)
	if err != nil {
		return modelSelection{}, "", err
	}
	if raw == "" {
		return modelSelection{}, "", fmt.Errorf("local engine returned an empty answer")
	}
	sel, err := parseSelection(raw)
	if err != nil {
		return modelSelection{}, "", err
	}
	return sel, out.Model, nil
}

// parseSelection extracts the JSON object the model was asked for. Models
// wrap JSON in prose or fences often enough that locating the outermost braces
// is worth more than a strict decode that fails on a stray "Here you go:".
func parseSelection(raw string) (modelSelection, error) {
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return modelSelection{}, fmt.Errorf("local engine did not return a finding selection")
	}
	var sel modelSelection
	if err := json.Unmarshal([]byte(raw[start:end+1]), &sel); err != nil {
		return modelSelection{}, fmt.Errorf("local engine returned an unparseable selection: %w", err)
	}
	return sel, nil
}
