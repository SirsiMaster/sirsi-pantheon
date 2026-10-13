package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGemmaSharedAmbiguousHeldCanceledNeverResend(t *testing.T) {
	for _, outcome := range []string{"OUTCOME UNKNOWN", "HELD in relay", "context canceled", "lost response across server hour"} {
		t.Run(outcome, func(t *testing.T) {
			home, key := t.TempDir(), strings.Repeat("d", 32)
			var out bytes.Buffer
			sends := 0
			send := func(title string) (string, error) {
				sends++
				if !strings.Contains(out.String(), key) {
					t.Fatal("key not printed before send")
				}
				return "", errors.New(outcome)
			}
			if _, err := sendSharedGemmaAttempt(home, "builder", key, "prompt", &out, send); err == nil {
				t.Fatal("lost send error")
			}
			if _, err := sendSharedGemmaAttempt(home, "builder", key, "prompt", &out, send); err == nil {
				t.Fatal("reuse admitted")
			}
			if sends != 1 {
				t.Fatalf("duplicate send: %d", sends)
			}
		})
	}
}

func TestGemmaSharedReceiptPreparedModeNoPromptAndNoRetry(t *testing.T) {
	home := t.TempDir()
	key := strings.Repeat("a", 32)
	prompt := "PRIVATE PROMPT DO NOT CACHE"
	r, err := prepareSharedGemmaReceipt(home, "builder", key, prompt)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(r.path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("initial mode: %v %v", info, err)
	}
	r.meta.State = "pending" // before a simulated timeout/HELD/canceled send
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	path := r.path
	r.close()
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), prompt) {
		t.Fatal("receipt contains prompt or unavailable", err)
	}
	for _, state := range []string{"pending", "prepared", "submitted"} {
		r, err = prepareSharedGemmaReceipt(home, "builder", key, prompt)
		if err != nil || !r.existing {
			t.Fatalf("retry accepted (%s): %v", state, err)
		}
		// Even a prepared/crashed file, a skewed timestamp, or an expired
		// hour cannot authorize resend. Existing is always a read-only result.
		r.close()
	}
	if _, err := prepareSharedGemmaReceipt(home, "builder", key, "different"); err == nil {
		t.Fatal("key/prompt conflict accepted")
	}
	if _, err := prepareSharedGemmaReceipt(home, "foreign", key, prompt); err == nil {
		t.Fatal("foreign actor reused key")
	}
}

func TestGemmaSharedReceiptSymlinkAndReplacementPreserved(t *testing.T) {
	home := t.TempDir()
	key := strings.Repeat("b", 32)
	r, err := prepareSharedGemmaReceipt(home, "builder", key, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	old := r.path + ".original"
	if err := os.Rename(r.path, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.persist(); err == nil {
		t.Fatal("substitution accepted")
	}
	data, _ := os.ReadFile(r.path)
	if string(data) != "replacement" {
		t.Fatal("replacement modified")
	}
	if err := os.Remove(r.path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(old, r.path); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareSharedGemmaReceipt(home, "builder", key, "prompt"); err == nil {
		t.Fatal("symlink accepted")
	}
}

func TestGemmaSharedReceiptClockAndClosedOriginalNeverAuthorizeSend(t *testing.T) {
	home := t.TempDir()
	key := strings.Repeat("c", 32)
	r, err := prepareSharedGemmaReceipt(home, "builder", key, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	r.meta.State = "submitted"
	r.meta.FirstSend = "1999-12-31T23:59:59Z"
	r.meta.OriginalID = "exact-closed-original"
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	r.close()
	r, err = prepareSharedGemmaReceipt(home, "builder", key, "prompt")
	if err != nil {
		t.Fatal(err)
	}
	defer r.close()
	if !r.existing || r.meta.OriginalID != "exact-closed-original" {
		t.Fatal("hour/clock changed receipt")
	}
	data, err := os.ReadFile(filepath.Join(home, ".sirsi", "gemma-shared-attempts", key+".json"))
	var fields map[string]any
	if err != nil || json.Unmarshal(data, &fields) != nil || len(fields) != 7 {
		t.Fatalf("not metadata-only: %s %v", data, err)
	}
}

func TestGemmaSharedPrivacyDisclosesRelayStaging(t *testing.T) {
	if !strings.Contains(sharedGemmaPrivacy, "relay may stage the prompt") || !strings.Contains(sharedGemmaPrivacy, "M5") {
		t.Fatal("misleading privacy disclosure")
	}
}
