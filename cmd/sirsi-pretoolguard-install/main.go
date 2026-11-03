// Command sirsi-pretoolguard-install implements a bounded macOS transaction.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

type identity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
}

type request struct {
	action, target, artifact, transaction, current, backup, replacement, identifier, team, originalProvenance string
}

type metadata struct {
	ProvenancePresent bool   `json:"provenance_present"`
	ProvenanceSHA256  string `json:"provenance_sha256"`
}

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var codeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,199}$`)

func metadataJSONValid(raw json.RawMessage) bool {
	var fields map[string]json.RawMessage
	var m metadata
	if json.Unmarshal(raw, &fields) != nil || fields["provenance_present"] == nil || fields["provenance_sha256"] == nil || string(fields["provenance_present"]) == "null" || string(fields["provenance_sha256"]) == "null" || json.Unmarshal(raw, &m) != nil {
		return false
	}
	return (m.ProvenancePresent && digest.MatchString(m.ProvenanceSHA256)) || (!m.ProvenancePresent && m.ProvenanceSHA256 == "")
}

func checkExpectedProvenance(raw string, observed metadata) error {
	var expected metadata
	if !metadataJSONValid([]byte(raw)) || json.Unmarshal([]byte(raw), &expected) != nil || observed != expected {
		return errors.New("original provenance does not match independent expectation")
	}
	return nil
}

func parseIdentity(s string) (identity, error) {
	var id identity
	if err := json.Unmarshal([]byte(s), &id); err != nil || id.Inode == 0 || id.Mode > 07777 || !digest.MatchString(id.SHA256) {
		return id, errors.New("invalid identity JSON")
	}
	return id, nil
}

func parse(args []string) (request, error) {
	var r request
	f := flag.NewFlagSet("sirsi-pretoolguard-install", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&r.action, "action", "", "install, rollback or recover")
	f.StringVar(&r.target, "target", "", "exact target selector")
	f.StringVar(&r.artifact, "artifact", "", "absolute signed replacement path")
	f.StringVar(&r.transaction, "transaction", "", "rollback transaction ID")
	f.StringVar(&r.current, "expected-current", "", "exact current identity JSON")
	f.StringVar(&r.backup, "expected-backup", "", "exact retained backup identity JSON")
	f.StringVar(&r.replacement, "expected-replacement-sha256", "", "signed artifact digest")
	f.StringVar(&r.identifier, "expected-code-identifier", "", "signed code identifier")
	f.StringVar(&r.team, "expected-team-id", "", "signing team")
	f.StringVar(&r.originalProvenance, "expected-original-provenance", "", "independent original provenance proof JSON")
	if f.Parse(args) != nil || f.NArg() != 0 {
		return r, errors.New("invalid arguments")
	}
	if r.target != "codex-user" && r.target != "claude-user" && r.target != "codex-system" {
		return r, errors.New("unknown target selector")
	}
	if _, err := parseIdentity(r.current); err != nil {
		return r, err
	}
	switch r.action {
	case "install":
		if r.artifact == "" || r.artifact[0] != '/' || !digest.MatchString(r.replacement) || !codeName.MatchString(r.identifier) || r.team != "9D382WV988" || r.transaction != "" || r.backup != "" || !metadataJSONValid([]byte(r.originalProvenance)) {
			return r, errors.New("invalid install parameters")
		}
	case "rollback", "recover":
		if !digest.MatchString(r.transaction) || r.artifact != "" || r.replacement != "" || r.identifier != "" || r.team != "" || r.originalProvenance != "" {
			return r, errors.New("invalid rollback parameters")
		}
		if _, err := parseIdentity(r.backup); err != nil {
			return r, err
		}
	default:
		return r, errors.New("action must be install, rollback or recover")
	}
	return r, nil
}

func main() {
	r, err := parse(os.Args[1:])
	if err == nil {
		err = execute(r)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
