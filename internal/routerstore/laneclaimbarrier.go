package routerstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const laneClaimBarrierKey = "lane-claim-barrier:gemma-pantheon"

var ErrLanePaused = errors.New("routerstore: lane claims paused for maintenance")

// Acquire requires a caller-generated random 128-bit hex capability, retained
// before sending. Retrying acquire with that same capability is idempotent,
// including after a lost HTTP response. Never serialize it into evidence.
type LaneClaimBarrierRequest struct {
	Action         string `json:"action"`
	Lane           string `json:"lane"`
	Token          string `json:"token,omitempty"`
	PreviousThread string `json:"previous_thread,omitempty"`
}

// Token is a controller capability. Retain only in operator memory, never logs.
// A barrier has no automatic expiry: a crashed controller cannot silently
// restart an old worker during replacement. Recovery requires its capability.
type LaneClaimBarrierResult struct {
	Token       string `json:"token,omitempty"`
	Paused      bool   `json:"paused"`
	ActiveItems int    `json:"active_items"`
	ActiveTasks int    `json:"active_tasks"`
}
type laneClaimBarrierState struct{ Owner, Digest string }
type laneClaimBarrierActorKey struct{}
type laneClaimBarrierActor struct{ Agent, Host, Thread string }

// lockLaneClaimsTx obtains the SAME durable row lock for pause and ALL claim
// paths. The no-op UPSERT takes a PostgreSQL row lock (not SKIP LOCKED) and
// SQLite's writer lock, even before the first pause. No schema migration.
func (s *SQLiteStore) lockLaneClaimsTx(tx *txHandle, agent string) (laneClaimBarrierState, error) {
	// Ordinary claim callers retain their existing context-free semantics.
	return s.lockLaneClaimsContextTx(context.Background(), tx, agent)
}

func (s *SQLiteStore) lockLaneClaimsContextTx(ctx context.Context, tx taskEvidenceTx, agent string) (laneClaimBarrierState, error) {
	var st laneClaimBarrierState
	if agent != "gemma-pantheon" {
		return st, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO state(key,value) VALUES (?, '{}') ON CONFLICT(key) DO UPDATE SET value=state.value`, laneClaimBarrierKey); err != nil {
		return st, err
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM state WHERE key=?`, laneClaimBarrierKey).Scan(&raw); err != nil {
		return st, err
	}
	err := json.Unmarshal([]byte(raw), &st)
	return st, err
}
func (s *SQLiteStore) laneClaimGateTx(tx *txHandle, agent string) error {
	st, err := s.lockLaneClaimsTx(tx, agent)
	if err != nil {
		return err
	}
	if st.Digest != "" {
		return ErrLanePaused
	}
	return nil
}
func barrierDigest(token string) string {
	b := sha256.Sum256([]byte(token))
	return hex.EncodeToString(b[:])
}

// LaneClaimBarrier is intentionally finite in scope: only registered SSA may
// maintain gemma-pantheon. The server derives identity; no caller actor fields.
// Status requires the same owner/capability and reads both lease tables in the
// locked transaction. Existing leases may renew/settle; expiry is NOT idle proof.
func (s *SQLiteStore) LaneClaimBarrier(ctx context.Context, req LaneClaimBarrierRequest) (LaneClaimBarrierResult, error) {
	var out LaneClaimBarrierResult
	a, ok := ctx.Value(laneClaimBarrierActorKey{}).(laneClaimBarrierActor)
	if !ok || a.Agent != "sirsi-software-admin" || a.Host == "" || a.Thread == "" || req.Lane != "gemma-pantheon" {
		return out, ErrLaneScope
	}
	owner := a.Agent + "/" + a.Host + "/" + a.Thread
	if req.Action != "acquire" && req.Action != "status" && req.Action != "release" && req.Action != "recover" {
		return out, fmt.Errorf("lane barrier: unknown action")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := s.retryWrite(func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		st, err := s.lockLaneClaimsContextTx(ctx, tx, req.Lane)
		if err != nil {
			return err
		}
		out = LaneClaimBarrierResult{}
		if req.Action == "recover" {
			// Exact owner recovery, never a time-based takeover: old controller must
			// have a terminal CLOSED registration on this same authenticated host.
			if req.PreviousThread == "" || req.PreviousThread == a.Thread {
				return ErrLeaseInvalid
			}
			var agent, host, status string
			if err := tx.QueryRowContext(ctx, `SELECT agent,host,status FROM threads WHERE thread_id=?`, req.PreviousThread).Scan(&agent, &host, &status); err != nil {
				return ErrLeaseInvalid
			}
			if agent != a.Agent || host != a.Host || status != "closed" {
				return ErrLeaseInvalid
			}
			decoded, e := hex.DecodeString(req.Token)
			if e != nil || len(decoded) != 16 {
				return ErrLeaseInvalid
			}
			digest := barrierDigest(req.Token)
			previous := a.Agent + "/" + a.Host + "/" + req.PreviousThread
			if st.Owner == owner && st.Digest == digest { // idempotent lost-response retry
			} else if st.Owner == previous && st.Digest != "" && st.Digest != digest {
				st = laneClaimBarrierState{owner, digest}
			} else {
				return ErrLeaseInvalid
			}
			out.Token = req.Token
		} else if req.Action == "acquire" {
			decoded, e := hex.DecodeString(req.Token)
			if e != nil || len(decoded) != 16 {
				return ErrLeaseInvalid
			}
			digest := barrierDigest(req.Token)
			if st.Digest != "" && (st.Owner != owner || st.Digest != digest) {
				return ErrLanePaused
			}
			st = laneClaimBarrierState{owner, digest}
			out.Token = req.Token

		} else if st.Digest == "" || st.Owner != owner || req.Token == "" || st.Digest != barrierDigest(req.Token) {
			return ErrLeaseInvalid
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE to_agent=? AND status IN ('claimed','working')`, req.Lane).Scan(&out.ActiveItems); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks WHERE agent=? AND (lease_token<>'' OR lease_expires<>'' OR claimed_by<>'' OR thread_id<>'')`, req.Lane).Scan(&out.ActiveTasks); err != nil {
			return err
		}
		out.Paused = true
		if req.Action == "release" {
			st = laneClaimBarrierState{}
			out.Paused = false
		}
		raw, err := json.Marshal(st)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE state SET value=? WHERE key=?`, string(raw), laneClaimBarrierKey); err != nil {
			return err
		}
		return tx.Commit()
	})
	if err != nil {
		return LaneClaimBarrierResult{}, err
	}
	return out, nil
}
