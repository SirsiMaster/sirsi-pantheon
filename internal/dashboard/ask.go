package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

type askResponse struct {
	Answer string `json:"answer"`
	Model  string `json:"model"`
	// Grounding names what the answer was allowed to be based on, so the UI can
	// say so out loud rather than presenting a model opinion as a measurement.
	Grounding string `json:"grounding"`
}

// groundingFromDoctor renders the live diagnostic report as the only facts the
// model may use.
func groundingFromDoctor(rpt *guard.DoctorReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Workstation health score: %d/100 (status: %s)\n", rpt.Score, rpt.Status)
	b.WriteString("Diagnostic findings:\n")
	sev := map[int]string{0: "OK", 1: "INFO", 2: "WARNING", 3: "CRITICAL"}
	for _, f := range rpt.Findings {
		label := sev[int(f.Severity)]
		if label == "" {
			label = "UNKNOWN"
		}
		fmt.Fprintf(&b, "- [%s] %s: %s\n", label, f.Check, f.Message)
		if f.Detail != "" {
			fmt.Fprintf(&b, "    detail: %s\n", f.Detail)
		}
	}
	return b.String()
}

const askSystemPrompt = `You are Horus, the local workstation monitor for Sirsi Pantheon.

Answer the operator's question USING ONLY the diagnostic report below. It is a
live reading of this specific machine taken seconds ago.

Rules:
- If the report does not contain the answer, say exactly what is missing and
  name the command that would produce it. Do not guess.
- Never invent a process name, a number, or a size. Every figure you state must
  appear verbatim in the report.
- Be brief and concrete. Two or three sentences. Lead with the answer.
- Plain language, no jargon.`

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

	answer, model, err := askLocalEngine(r.Context(), req.Question, groundingFromDoctor(report))
	if err != nil {
		// Fail loud. A degraded answer here would be indistinguishable from a
		// real one, which is the failure mode this whole surface exists to avoid.
		writeError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, askResponse{
		Answer:    answer,
		Model:     model,
		Grounding: fmt.Sprintf("%d live diagnostic findings from this machine", len(report.Findings)),
	})
}

// askLocalEngine sends one grounded completion to the loopback SNE server.
func askLocalEngine(ctx context.Context, question, grounding string) (string, string, error) {
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
		return "", "", fmt.Errorf("build request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", "", fmt.Errorf("local engine not reachable on 127.0.0.1:%d — is ai.sirsi.gemma-broker running? (%v)", port, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("local engine returned %s", resp.Status)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("local engine sent an unreadable response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", "", fmt.Errorf("local engine returned no choices")
	}
	answer := cleanCompletion(out.Choices[0].Message.Content)
	if answer == "" {
		return "", "", fmt.Errorf("local engine returned an empty answer")
	}
	return answer, out.Model, nil
}

// controlToken matches the chat-template markers this model family emits
// inline: <|channel>, <channel|>, <turn|>, <start_of_turn>, <end_of_turn>.
// Restricted to lowercase and underscores, which is what those token
// vocabularies use, so ordinary prose containing angle brackets survives.
var controlToken = regexp.MustCompile(`<\|?[a-z_]+\|?>`)

// cleanCompletion strips chat-template scaffolding from raw model output.
//
// gemma-4-12b-it-8bit answers through a channel protocol and returns it
// verbatim in the completion — a correct answer arrived as
// "<|channel>thought\n<channel|>The top consumer is …<turn|>". Rendering that
// to an operator is indistinguishable from a broken surface.
//
// The answer is whatever follows the LAST channel header; anything before it
// is the model's scratch channel and must not reach the screen as if it were
// the response.
func cleanCompletion(s string) string {
	const chanClose = "<channel|>"
	if i := strings.LastIndex(s, chanClose); i >= 0 {
		s = s[i+len(chanClose):]
	}
	return strings.TrimSpace(controlToken.ReplaceAllString(s, ""))
}
