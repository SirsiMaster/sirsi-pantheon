package fixture

import (
	"context"

	"cloud.google.com/go/firestore"
)

// Class F: form answers written to the document root a viewer role can read.
func store(ctx context.Context, ref *firestore.DocumentRef, ssn string) error {
	_, err := ref.Create(ctx, map[string]any{
		"title": "Petition",
		"ssn":   ssn, // want F
	})
	return err
}
