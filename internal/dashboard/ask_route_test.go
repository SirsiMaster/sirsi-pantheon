package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/engine"
)

type askPromptExecutor struct {
	completion engine.Completion
	receipt    engine.Receipt
}

func (f askPromptExecutor) CompletePrompt(context.Context, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	return f.completion, f.receipt, nil
}

type askControllerWithoutReceipt struct{}

func (askControllerWithoutReceipt) Snapshot() engine.SelectionSnapshot {
	return engine.SelectionSnapshot{}
}
func (askControllerWithoutReceipt) Select(engine.RoutePolicy) (engine.SelectionSnapshot, error) {
	return engine.SelectionSnapshot{}, nil
}
func (askControllerWithoutReceipt) CompletePrompt(context.Context, engine.PromptRequest) (engine.Completion, engine.Receipt, error) {
	return engine.Completion{Model: "local", Text: `{"findings":[],"summary":"local answer"}`}, engine.Receipt{}, nil
}

func TestAPIAskFailsClosedWithoutCanonicalPromptController(t *testing.T) {
	server := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"what should I address?"}`))
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusServiceUnavailable, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "canonical engine selection and receipt controller is not configured") {
		t.Fatalf("missing-controller response = %s", resp.Body.String())
	}
}

func TestAPIAskRejectsAnswerWithoutVerifiableRouteReceipt(t *testing.T) {
	server := New(Config{EngineSelection: askControllerWithoutReceipt{}})
	req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(`{"question":"what should I address?"}`))
	resp := httptest.NewRecorder()
	server.apiAsk(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusServiceUnavailable, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "selected engine returned no verifiable route receipt") {
		t.Fatalf("missing-receipt response = %s", resp.Body.String())
	}
}

func TestAskSelectedEngineReturnsRouteBoundReceipt(t *testing.T) {
	executor := askPromptExecutor{
		completion: engine.Completion{Model: "sne-model", Text: `{"findings":[0],"summary":"the selected engine answered"}`},
		receipt: engine.Receipt{
			RequestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Route:         &engine.RouteDecision{Requested: engine.KindSNE, Selected: engine.KindSNE, Rationale: "preferred sne connector admitted"},
		},
	}
	selection, model, receipt, err := askSelectedEngine(context.Background(), executor, "what is healthy?", "[0] OK health")
	if err != nil {
		t.Fatal(err)
	}
	if model != "sne-model" || len(selection.Findings) != 1 || receipt == nil {
		t.Fatalf("selection result = %+v, model=%q, receipt=%+v", selection, model, receipt)
	}
	if receipt.Route == nil || receipt.Route.Selected != engine.KindSNE || receipt.RequestSHA256 == "" {
		t.Fatalf("receipt lost route/request identity: %+v", receipt)
	}
}
