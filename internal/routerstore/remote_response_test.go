package routerstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteResponseFrameBoundAndShape(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		limit      int64
		wantErr    bool
	}{
		{"void", `{}`, 128, false},
		{"list", `{"result":[[]]}`, 128, false},
		{"empty-list-null", `{"result":[null]}`, 128, false},
		{"future-field", `{"result":[[]],"future":true}`, 128, false},
		{"whitespace", " \n{\"result\":[[]]}\n", 128, false},
		{"legal-json-padding", "\t\r\n {} \r\n\t", 128, false},
		{"vertical-tab-padding", "\v{}\v", 128, true},
		{"form-feed-padding", "\f{}\f", 128, true},
		{"nbsp-padding", "\u00a0{}\u00a0", 128, true},
		{"empty", "", 128, true},
		{"only-whitespace", " \n\t", 128, true},
		{"null", `null`, 128, true},
		{"array", `[]`, 128, true},
		{"html", `<html>private-marker</html>`, 128, true},
		{"truncated", `{"result":[[`, 128, true},
		{"extra-object", `{"result":[[]]}{}`, 128, true},
		{"extra-output", "{\"result\":[[]]}\nprivate-marker", 128, true},
		{"wrong-result-type", `{"result":true}`, 128, true},
		{"exact-bound", `{}` + strings.Repeat(" ", 30), 32, false},
		{"valid-prefix-hidden-tail", `{}` + strings.Repeat(" ", 30) + "private-marker", 32, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := strings.NewReader(tc.body)
			_, err := readRPCResponse(r, tc.limit)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tc.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "private-marker") {
				t.Fatal("private response content escaped into diagnostics")
			}
			if int64(len(tc.body)-r.Len()) > tc.limit+1 {
				t.Fatal("reader exceeded bounded overflow probe")
			}
		})
	}
}

type responseReadFailure struct{}

func (responseReadFailure) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestRemoteResponseFrameReadFailure(t *testing.T) {
	if _, err := readRPCResponse(responseReadFailure{}, 128); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("want retained read failure, got %v", err)
	}
}

// Exercise the actual HTTP consumer without sessions, SQLite, credentials,
// canonical endpoints, retries, mutations or an operator's durable snapshot.
func TestRemoteResponseHTTPPreservesCallerStateAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		void       bool
		wantErr    bool
		sentinel   error
	}{
		{"list", `{"result":[[{"task_id":"fixture-task"}]]}`, 200, false, false, nil},
		{"void", `{}`, 200, true, false, nil},
		{"legal-json-padding-void", "\t\r\n {} \r\n\t", 200, true, false, nil},
		{"vertical-tab-padding-void", "\v{}\v", 200, true, true, nil},
		{"form-feed-padding-void", "\f{}\f", 200, true, true, nil},
		{"nbsp-padding-void", "\u00a0{}\u00a0", 200, true, true, nil},
		{"empty-void", ``, 200, true, true, nil},
		{"null-void", `null`, 200, true, true, nil},
		{"html502", `<html>private-marker</html>`, 502, false, true, nil},
		{"html200", `<html>private-marker</html>`, 200, false, true, nil},
		{"truncated", `{"result":[[`, 200, false, true, nil},
		{"extra-output", `{"result":[[]]} private-marker`, 200, false, true, nil},
		{"wrong-result-arity", `{"result":[]}`, 200, false, true, nil},
		{"http-error-json", `{"result":[[]]}`, 502, false, true, nil},
		{"sentinel", `{"error":{"name":"ErrNotFound","message":"missing"}}`, 200, false, true, ErrNotFound},
		{"service-unavailable", `{"error":{"message":"temporarily unavailable"}}`, 503, false, true, ErrServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.Method != http.MethodPost || r.URL.Path != "/v1/call/ListTasks" {
					t.Errorf("unexpected route %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			rs := &RemoteStore{base: srv.URL, client: srv.Client()}
			lastGood := []Task{{TaskID: "retained-last-good"}}
			var err error
			if tc.void {
				err = rs.do(context.Background(), "ListTasks", nil, nil)
			} else {
				err = rs.do(context.Background(), "ListTasks", nil, nil, &lastGood)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Fatalf("lost sentinel %v: %v", tc.sentinel, err)
			}
			if err != nil && (len(lastGood) != 1 || lastGood[0].TaskID != "retained-last-good") {
				t.Fatal("failed frame changed caller's last-good value")
			}
			if err == nil && !tc.void && (len(lastGood) != 1 || lastGood[0].TaskID != "fixture-task") {
				t.Fatalf("positive task not decoded: %+v", lastGood)
			}
			if err != nil && strings.Contains(err.Error(), "private-marker") {
				t.Fatal("private HTTP body escaped into diagnostics")
			}
			if requests != 1 {
				t.Fatalf("unexpected retry count %d", requests)
			}
		})
	}
}
