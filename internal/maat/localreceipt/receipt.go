// Package localreceipt validates ordinary local execution facts, never remote
// grants, model admission, privileged management or a Ra session.
package localreceipt

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

const Schema = "sirsi.maat.local-execution.v1"

type Fact struct {
	Schema          string    `json:"schema"`
	Scope           string    `json:"scope"`
	UID             int       `json:"uid"`
	SupervisorPID   int       `json:"supervisor_pid"`
	SupervisorStart string    `json:"supervisor_start"`
	ArgvSHA256      string    `json:"argv_sha256"`
	StartedAt       time.Time `json:"started_at"`
	Deadline        time.Time `json:"deadline"`
	CPUCores        int       `json:"cpu_cores"`
}

// Allowed is the authoritative closed local operation set. The supervisor's
// candidate check only routes; an unsupported flag/operation fails here.
func Allowed(args []string) bool {
	var flags []string
	switch {
	case len(args) > 0 && (args[0] == "version" || args[0] == "vitals" || args[0] == "diagnose"):
		flags = args[1:]
	case len(args) > 1 && args[0] == "maat" && (args[1] == "triage" || args[1] == "decisions" || args[1] == "casebook"):
		flags = args[2:]
	case len(args) > 1 && args[0] == "thoth" && (args[1] == "status" || args[1] == "compact" || args[1] == "sync" || args[1] == "init"):
		flags = args[2:]
	case len(args) > 2 && args[0] == "stacklab" && args[1] == "local" && (args[2] == "inspect" || args[2] == "enable" || args[2] == "disable" || args[2] == "catalog"):
		flags = args[3:]
	case len(args) > 2 && args[0] == "liveness-watch" && args[1] == "run" && args[2] == "--local":
		flags = args[3:]
	case len(args) > 1 && args[0] == "liveness-watch" && args[1] == "status":
		flags = args[2:]
	default:
		return false
	}
	for i := 0; i < len(flags); i++ {
		switch flags[i] {
		case "--json":
		case "--confirm":
			if !(len(args) > 1 && args[0] == "maat" && args[1] == "triage") {
				return false
			}
		case "--summary":
			if args[0] != "vitals" {
				return false
			}
		case "--provider":
			if len(args) < 3 || args[0] != "stacklab" || args[2] != "enable" || i+1 >= len(flags) {
				return false
			}
			i++
			if flags[i] != "local" && flags[i] != "frontier" && flags[i] != "self-hosted" {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func Validate(data []byte, uid, parent int, birth string, argv []string, now time.Time) error {
	var f Fact
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("local admission: trailing receipt data")
	}
	hash := sha256.Sum256([]byte(strings.Join(argv, "\x00")))
	if uid <= 0 || len(argv) < 2 || !Allowed(argv[1:]) || f.Schema != Schema || f.Scope != "local" || f.UID != uid || parent <= 1 || birth == "" || f.SupervisorPID != parent || f.SupervisorStart != birth || f.ArgvSHA256 != hex.EncodeToString(hash[:]) || f.CPUCores != 1 || f.StartedAt.After(now.Add(5*time.Second)) || !f.Deadline.After(now) || f.Deadline.Sub(f.StartedAt) <= 0 || f.Deadline.Sub(f.StartedAt) > 2*time.Hour {
		return errors.New("local admission: exact parent/start/argv/UID/deadline or closed Tier1 operation differs")
	}
	return nil
}

type verifiedKey struct{}

func Verified(ctx context.Context) bool { value, _ := ctx.Value(verifiedKey{}).(bool); return value }

// Consume removes the direct-child fact before descendants run. It never sets
// an offline mode or changes router identity/configuration/remote authorization.
func Consume(ctx context.Context, argv []string) (context.Context, error) {
	path := os.Getenv("SIRSI_MAAT_LOCAL_EXECUTION_RECEIPT")
	if path == "" {
		return ctx, nil
	}
	if os.Getuid() == 0 || os.Geteuid() != os.Getuid() || os.Getgid() != os.Getegid() {
		return ctx, errors.New("local admission: ordinary matching process identities required")
	}
	data, err := readFact(path)
	if err != nil {
		return ctx, err
	}
	if err = Validate(data, os.Getuid(), os.Getppid(), parentStart(os.Getppid()), argv, time.Now()); err != nil {
		return ctx, err
	}
	if err = os.Unsetenv("SIRSI_MAAT_LOCAL_EXECUTION_RECEIPT"); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, verifiedKey{}, true), nil
}
