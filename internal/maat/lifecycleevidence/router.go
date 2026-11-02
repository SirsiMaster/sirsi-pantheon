// Package lifecycleevidence resolves exact canonical router bytes and, only
// when constructed through the installed signature verifier, the embedded
// release acceptance inventory.
package lifecycleevidence

import (
	"context"
	"fmt"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type ItemReader interface {
	Get(string) (routerstore.Item, error)
}

type Router struct {
	Reader    ItemReader
	authority maat.InstalledEngineAuthority
}

// ResolveLifecycleRaw returns the exact Instructions bytes from the selected
// closed canonical item. From remains custody attribution, never identity proof.
func (r Router) ResolveLifecycleRaw(ctx context.Context, id string) (maat.LifecycleRawRecord, error) {
	var result maat.LifecycleRawRecord
	if id == "" || len(id) > 512 {
		return result, fmt.Errorf("select an exact canonical receipt ID")
	}
	if r.authority.Valid() {
		if embedded, ok := maat.InstalledLifecycleRawRecord(id); ok {
			return embedded, nil
		}
	}
	if r.Reader == nil {
		return result, fmt.Errorf("canonical lifecycle receipt is not embedded and no canonical reader is available")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	item, err := r.Reader.Get(id)
	if err != nil {
		return result, fmt.Errorf("canonical receipt unavailable: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if item.ID != id || item.Status != "closed" || (item.Type != "review" && item.Type != "decision") || strings.TrimSpace(item.From) == "" || len(item.From) > 512 {
		return result, fmt.Errorf("select a closed canonical review/decision with sender-authored Instructions; no Result identity borrowing")
	}
	return maat.LifecycleRawRecord{ID: item.ID, Issuer: item.From, RawInstructions: []byte(item.Instructions)}, nil
}

func (r Router) ResolveLifecycle(ctx context.Context, id string) (maat.LifecycleEvidence, error) {
	record, err := r.ResolveLifecycleRaw(ctx, id)
	if err != nil {
		return maat.LifecycleEvidence{}, err
	}
	var evidence maat.LifecycleEvidence
	if r.authority.Valid() {
		evidence, err = maat.VerifyInstalledLifecycleRecord(ctx, r.authority, record, r)
	} else {
		evidence, err = maat.DecodeLifecycleCustodyRecord(record)
	}
	if err != nil {
		return maat.LifecycleEvidence{}, fmt.Errorf("selected Instructions are not strict %s: %w; choose structured raw evidence, not a title or returned prose", maat.LifecycleSchema, err)
	}
	return evidence, nil
}
