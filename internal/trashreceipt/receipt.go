// Package trashreceipt implements the native, review-bound Empty Trash path.
// It is not an atomic stat/unlink defense against a hostile same-UID writer.
package trashreceipt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	ManifestSchema   = "pantheon.trash-manifest.v1"
	ReportSchema     = "pantheon.trash-report.v1"
	MaxEntries       = 1024
	MaxManifestBytes = 64 * 1024
	MaxDirectories   = 64
	MaxDepth         = 24
)

type Identity struct {
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	UID     uint32 `json:"uid"`
	Mode    uint32 `json:"mode"`
	Size    int64  `json:"size"`
	MtimeNS int64  `json:"mtime_ns"`
	CtimeNS int64  `json:"ctime_ns"`
	Type    string `json:"type"`
}

type Entry struct {
	Path     string   `json:"path"`
	Identity Identity `json:"identity"`
}

type Manifest struct {
	Schema     string   `json:"schema"`
	UID        uint32   `json:"uid"`
	Root       Identity `json:"root"`
	Containers []Entry  `json:"containers"`
	Entries    []Entry  `json:"entries"`
}

type Report struct {
	Schema       string    `json:"schema"`
	Status       string    `json:"status"`
	Preview      *Manifest `json:"preview,omitempty"`
	Attempted    int       `json:"attempted"`
	Deleted      []string  `json:"deleted"`
	LogicalBytes int64     `json:"logical_bytes"`
	Errors       []string  `json:"errors"`
}

func report(status string) Report {
	return Report{Schema: ReportSchema, Status: status, Deleted: []string{}, Errors: []string{}}
}

func Failure(err error) Report {
	r := report("failed")
	r.Errors = append(r.Errors, err.Error())
	return r
}

// Decode consumes the entire bounded input, rejects duplicate/unknown keys,
// and validates every member before any filesystem or deletion authority.
func Decode(r io.Reader) (Manifest, error) {
	var m Manifest
	err := decodeJSON(r, &m, requiredManifestFields)
	if err != nil {
		return m, err
	}
	return m, Validate(m)
}

func decodeJSON(r io.Reader, target any, fields func([]byte) error) error {
	raw, err := io.ReadAll(io.LimitReader(r, MaxManifestBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > MaxManifestBytes {
		return fmt.Errorf("Trash request exceeds the 64 KiB input bound; choose a smaller batch")
	}
	keys := json.NewDecoder(bytes.NewReader(raw))
	if err := uniqueJSON(keys, 0); err != nil {
		return err
	}
	if _, err := keys.Token(); err != io.EOF {
		return fmt.Errorf("Trash request has trailing data")
	}
	if err := fields(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("invalid Trash request: %w", err)
	}
	return nil
}

func closedObject(raw []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	// encoding/json accepts case-insensitive struct field aliases, even
	// with DisallowUnknownFields. Count plus exact required-name lookup
	// establishes the closed key set before decoding into those structs.
	if len(fields) != len(names) {
		return nil, fmt.Errorf("Trash review object must contain exactly its canonical keys")
	}
	for _, name := range names {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("missing Trash review field %s", name)
		}
	}
	return fields, nil
}

func requiredManifestFields(raw []byte) error {
	identityFields := []string{"device", "inode", "uid", "mode", "size", "mtime_ns", "ctime_ns", "type"}
	m, err := closedObject(raw, "schema", "uid", "root", "containers", "entries")
	if err != nil {
		return err
	}
	if _, err := closedObject(m["root"], identityFields...); err != nil {
		return err
	}
	for _, key := range []string{"containers", "entries"} {
		var entries []json.RawMessage
		if err := json.Unmarshal(m[key], &entries); err != nil {
			return err
		}
		for _, rawEntry := range entries {
			e, err := closedObject(rawEntry, "path", "identity")
			if err != nil {
				return err
			}
			if _, err := closedObject(e["identity"], identityFields...); err != nil {
				return err
			}
		}
	}
	return nil
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 12 {
		return fmt.Errorf("Trash review JSON nesting exceeds bound")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("invalid Trash review JSON")
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return fmt.Errorf("duplicate Trash review key")
			}
			seen[s] = true
		}
		if err := uniqueJSON(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func validIdentity(i Identity) bool {
	want := map[string]uint32{"file": 0o100000, "directory": 0o040000, "symlink": 0o120000}
	t, ok := want[i.Type]
	return ok && i.Inode != 0 && i.Size >= 0 && i.Mode&0o170000 == t
}

func Validate(m Manifest) error {
	if m.Schema != ManifestSchema || !validIdentity(m.Root) || m.Root.Type != "directory" || m.Root.UID != m.UID || m.Entries == nil || m.Containers == nil || len(m.Entries) > MaxEntries || len(m.Containers) > MaxDirectories {
		return fmt.Errorf("invalid Trash manifest schema, root or entry bound")
	}
	parents := map[string]bool{".": true}
	containerPaths := map[string]bool{}
	previousContainer := ""
	for _, e := range m.Containers {
		if !validRelative(e.Path) || e.Path <= previousContainer || e.Identity.Type != "directory" || !validIdentity(e.Identity) || e.Identity.UID != m.UID || e.Identity.Device != m.Root.Device || containerPaths[e.Path] || !parents[path.Dir(e.Path)] {
			return fmt.Errorf("invalid container identity or ancestry")
		}
		parents[e.Path] = true
		containerPaths[e.Path] = true
		previousContainer = e.Path
	}
	previous := ""
	dirs := len(m.Containers)
	var total int64
	for _, e := range m.Entries {
		if !validRelative(e.Path) || containerPaths[e.Path] || e.Path <= previous || !parents[path.Dir(e.Path)] || !validIdentity(e.Identity) || e.Identity.UID != m.UID || e.Identity.Device != m.Root.Device {
			return fmt.Errorf("invalid, duplicate, unordered or unowned Trash member")
		}
		previous = e.Path
		if e.Identity.Type == "directory" {
			parents[e.Path] = true
			dirs++
			if dirs > MaxDirectories {
				return fmt.Errorf("Trash directory bound exceeded")
			}
		} else {
			if e.Identity.Size > math.MaxInt64-total {
				return fmt.Errorf("Trash logical size overflow")
			}
			total += e.Identity.Size
		}
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > MaxManifestBytes {
		return fmt.Errorf("Trash manifest exceeds serialized byte bound")
	}
	return nil
}

func validRelative(p string) bool {
	return p != "" && p != "." && p != ".." && utf8.ValidString(p) && len(p) <= 8192 && !path.IsAbs(p) && path.Clean(p) == p && !strings.HasPrefix(p, "../") && !strings.ContainsRune(p, 0) && strings.Count(p, "/") < MaxDepth
}
