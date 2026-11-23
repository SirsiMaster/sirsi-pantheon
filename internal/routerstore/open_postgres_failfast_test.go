package routerstore

import (
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A database that accepts the connection and then never answers is the shape of
// the 2026-11-11 Cloud SQL restart (the Cloud Run proxy held the socket for its
// own 30 s dial timeout). Inbox takes no context, so only the connect timeout
// can end it: it must fail within the bound and surface as a retryable 503.
func TestUnresponsiveDatabaseFailsFastAs503(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			held = append(held, c) // accept, never speak
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		for _, c := range held {
			_ = c.Close()
		}
	})

	old := postgresConnectTimeout
	postgresConnectTimeout = time.Second
	t.Cleanup(func() { postgresConnectTimeout = old })

	db, err := sql.Open("pgx", postgresDSN("postgres://u@"+ln.Addr().String()+"/d?sslmode=disable"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	st := &SQLiteStore{db: &dbHandle{db: db, d: postgresDialect}, d: postgresDialect}

	type res struct {
		err error
		dur time.Duration
	}
	done := make(chan res, 1)
	start := time.Now()
	go func() {
		_, err := st.Inbox("anyone")
		done <- res{err, time.Since(start)}
	}()
	var r res
	select {
	case r = <-done:
	case <-time.After(5 * time.Second): // own deadline: never hang the suite
		t.Fatal("Inbox against an unresponsive database still blocked after 5 s; connect is unbounded")
	}
	if r.err == nil {
		t.Fatal("Inbox against an unresponsive database returned nil error")
	}
	if r.dur > 3*time.Second {
		t.Fatalf("Inbox took %s, want about the 1 s connect timeout", r.dur)
	}

	rec := httptest.NewRecorder()
	(&server{}).writeCallErr(rec, "Inbox", r.err)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503 for a failed connect; err=%v", rec.Code, r.err)
	}
	if body := rec.Body.String(); !strings.Contains(body, "database unreachable") || strings.Contains(body, ln.Addr().String()) {
		t.Fatalf("503 body should be a fixed retryable message without the target: %s", body)
	}
}

// A query that ran and failed is still a 500, and an operator-set
// connect_timeout is never overridden.
func TestWriteCallErrKeepsQueryFailures500AndDSNOverride(t *testing.T) {
	rec := httptest.NewRecorder()
	(&server{}).writeCallErr(rec, "Inbox", sql.ErrNoRows)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500 for a non-connect failure", rec.Code)
	}
	if got := postgresDSN("postgres://u@h/d?connect_timeout=9"); strings.Count(got, "connect_timeout") != 1 || !strings.Contains(got, "connect_timeout=9") {
		t.Fatalf("operator connect_timeout overridden: %s", got)
	}
	if got := postgresDSN("postgres://u@h/d"); !strings.Contains(got, "search_path=router") || !strings.Contains(got, "connect_timeout=5") {
		t.Fatalf("defaults missing: %s", got)
	}
}
