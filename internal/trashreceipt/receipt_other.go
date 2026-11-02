//go:build !darwin

package trashreceipt

import "fmt"

func Preview() Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}
func Apply(Manifest, bool) Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}

func Browse(BrowseRequest) Page {
	return pageFailure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}
func Select(SelectionRequest) Report {
	return Failure(fmt.Errorf("review-bound Empty Trash is supported only on macOS"))
}

func ReadHistoryPage(int, string) HistoryPage {
	return HistoryPage{Schema: HistoryPageSchema, Items: []HistoryItem{}, Incomplete: true, Issues: []string{"durable Empty Trash history is supported only on macOS"}}
}

func ReadHistoryDetail(id string) HistoryDetail {
	return HistoryDetail{Schema: HistoryDetailSchema, OperationID: id, State: "unknown", Attempts: []HistoryAttempt{}, IntegrityError: "durable Empty Trash history is supported only on macOS"}
}
