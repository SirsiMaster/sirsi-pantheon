package dashboard

import (
	"context"
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
