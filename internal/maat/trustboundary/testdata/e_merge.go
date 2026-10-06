package fixture

import (
	"context"

	"cloud.google.com/go/firestore"
)

// Class E: MergeAll keeps stale PII keys from the previous version.
func save(ctx context.Context, ref *firestore.DocumentRef, answers map[string]any) error {
	_, err := ref.Set(ctx, map[string]any{"answers": answers}, firestore.MergeAll) // want E F
	if err != nil {
		return err
	}
	data := map[string]any{"a": 1}
	_, err = ref.Set(ctx, data, firestore.MergeAll) // want E
	return err
}
