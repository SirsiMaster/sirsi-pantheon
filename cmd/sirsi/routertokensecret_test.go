package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

type secretRoundTrip func(*http.Request) (*http.Response, error)

func (fn secretRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

type secretFailedWriter struct{}

func (secretFailedWriter) Write([]byte) (int, error) { return 0, errors.New("writer failed") }

func TestSSAScopedTokenSecretReceiptNeverPrintsCredential(t *testing.T) {
	var out bytes.Buffer
	revoked := false
	rec := routerstore.HostToken{ID: "token-id", Host: "ssa-cloud", Agents: []string{"sirsi-software-admin"}}
	resource := "projects/210890802638/secrets/ssa-router-cloud"
	err := publishMintedRouterToken(context.Background(), &out, "sentinel-secret", rec, resource,
		func(_ context.Context, got, value string) (string, error) {
			if got != resource || value != "sentinel-secret" {
				t.Fatal("publication mismatch")
			}
			return resource + "/versions/1", nil
		}, func(string) error { revoked = true; return nil })
	if err != nil || revoked || strings.Contains(out.String(), "sentinel-secret") || !strings.Contains(out.String(), "token-id") {
		t.Fatalf("invalid receipt/revocation: %v %t", err, revoked)
	}
}

func TestSSAScopedTokenSecretFailuresRevokeAndReportDebt(t *testing.T) {
	for _, mode := range []string{"publish", "writer", "scope", "revoke-debt"} {
		t.Run(mode, func(t *testing.T) {
			rec := routerstore.HostToken{ID: "token-id", Host: "ssa-cloud", Agents: []string{"sirsi-software-admin"}}
			if mode == "scope" {
				rec.Agents = nil
			}
			var out bytes.Buffer
			var writer io.Writer = &out
			if mode == "writer" {
				writer = secretFailedWriter{}
			}
			revoked := false
			err := publishMintedRouterToken(context.Background(), writer, "sentinel-secret", rec, "projects/210890802638/secrets/ssa-router-cloud",
				func(context.Context, string, string) (string, error) {
					if mode == "publish" || mode == "revoke-debt" {
						return "", errors.New("sentinel-secret")
					}
					return "projects/210890802638/secrets/ssa-router-cloud/versions/1", nil
				},
				func(id string) error {
					if id != "token-id" {
						t.Fatal("wrong revoke")
					}
					revoked = true
					if mode == "revoke-debt" {
						return errors.New("sentinel-secret")
					}
					return nil
				})
			if err == nil || !revoked || strings.Contains(err.Error()+out.String(), "sentinel-secret") {
				t.Fatalf("not fail closed/redacted: %v", err)
			}
			if (mode == "revoke-debt") != strings.Contains(err.Error(), "CLEANUP_DEBT") {
				t.Fatal("debt classification")
			}
		})
	}
}

func TestSSAScopedTokenSecretNativePublication(t *testing.T) {
	resource := "projects/210890802638/secrets/ssa-router-cloud"
	for _, mode := range []string{"success", "identity", "publication", "wrong-version", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }, Transport: secretRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				status := 200
				header := make(http.Header)
				body := ""
				if calls == 1 {
					if r.URL.Host != "metadata.google.internal" || r.Header.Get("Metadata-Flavor") != "Google" {
						t.Fatal("wrong identity endpoint")
					}
					header.Set("Metadata-Flavor", "Google")
					body = `{"access_token":"identity-sentinel","token_type":"Bearer","expires_in":300}`
					if mode == "identity" {
						status = 403
					}
				} else {
					if calls != 2 || r.URL.Scheme != "https" || r.URL.Host != "secretmanager.googleapis.com" || r.URL.Path != "/v1/"+resource+":addVersion" || r.Header.Get("Authorization") != "Bearer identity-sentinel" {
						t.Fatal("unbound publication endpoint")
					}
					var p struct {
						Payload struct {
							Data string `json:"data"`
						} `json:"payload"`
					}
					if json.NewDecoder(r.Body).Decode(&p) != nil {
						t.Fatal("bad payload")
					}
					b, e := base64.StdEncoding.DecodeString(p.Payload.Data)
					if e != nil || string(b) != "credential-sentinel" {
						t.Fatal("bad token")
					}
					body = `{"name":"` + resource + `/versions/1"}`
					if mode == "publication" {
						status = 403
						body = `{"message":"credential-sentinel"}`
					}
					if mode == "wrong-version" {
						body = `{"name":"projects/999999/secrets/foreign/versions/1"}`
					}
					if mode == "redirect" {
						status = 307
						header.Set("Location", "https://untrusted.invalid")
					}
				}
				return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			name, err := publishScopedRouterSecretWithClient(context.Background(), resource, "credential-sentinel", client)
			if mode == "success" {
				if err != nil || name != resource+"/versions/1" {
					t.Fatalf("success: %v", err)
				}
			} else if err == nil || strings.Contains(err.Error(), "sentinel") {
				t.Fatalf("unsafe failure: %v", err)
			}
		})
	}
}
