package trashreceipt

import (
	"encoding/json"
	"fmt"
	"io"
)

const BrowseSchema = "pantheon.trash-browse.v2"
const SelectionSchema = "pantheon.trash-selection.v1"
const PageSize = 32
const DirectoryChunkBytes = 4096
const MaxBrowseRows = 128

type BlockedEntry struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Type   string `json:"type"`
}

type BrowseWork struct {
	NativeCalls int `json:"native_calls"`
	Bytes       int `json:"bytes"`
	Inspected   int `json:"inspected"`
}

type directoryChunk struct {
	Names []string
	Next  uint64
	EOF   bool
	Bytes int
	Calls int
}

// A continuation is read-only position/freshness metadata, never delete consent.
type BrowseRequest struct {
	Schema     string    `json:"schema"`
	Directory  string    `json:"directory"`
	Position   uint64    `json:"position"`
	Generation string    `json:"generation"`
	Root       *Identity `json:"root"`
}

type Page struct {
	Schema     string         `json:"schema"`
	Status     string         `json:"status"`
	UID        uint32         `json:"uid"`
	Root       Identity       `json:"root"`
	Directory  string         `json:"directory"`
	Containers []Entry        `json:"containers"`
	Entries    []Entry        `json:"entries"`
	Blocked    []BlockedEntry `json:"blocked"`
	Work       BrowseWork     `json:"work"`
	Generation string         `json:"generation"`
	More       bool           `json:"more"`
	Next       *BrowseRequest `json:"next"`
	Errors     []string       `json:"errors"`
}

type SelectionRequest struct {
	Schema     string   `json:"schema"`
	UID        uint32   `json:"uid"`
	Root       Identity `json:"root"`
	Containers []Entry  `json:"containers"`
	Entries    []Entry  `json:"entries"`
}

func DecodeBrowse(r io.Reader) (BrowseRequest, error) {
	var q BrowseRequest
	err := decodeJSON(r, &q, func(raw []byte) error {
		fields, err := closedObject(raw, "schema", "directory", "position", "generation", "root")
		// root is explicitly null on the initial browse only.
		if err != nil {
			var m map[string]json.RawMessage
			if e := json.Unmarshal(raw, &m); e != nil {
				return e
			}
			if len(m) != 5 || string(m["root"]) != "null" {
				return err
			}
			for _, key := range []string{"schema", "directory", "position", "generation"} {
				if len(m[key]) == 0 || string(m[key]) == "null" {
					return err
				}
			}
			return nil
		}
		_, err = closedObject(fields["root"], "device", "inode", "uid", "mode", "size", "mtime_ns", "ctime_ns", "type")
		return err
	})
	if err != nil {
		return q, err
	}
	if q.Schema != BrowseSchema || q.Directory != "." && !validRelative(q.Directory) || q.Generation != "" && (len(q.Generation) != 64 || q.Root == nil) || q.Position != 0 && (q.Generation == "" || q.Root == nil) {
		return q, fmt.Errorf("invalid browse position")
	}
	return q, nil
}

func DecodeSelection(r io.Reader) (SelectionRequest, error) {
	var q SelectionRequest
	err := decodeJSON(r, &q, requiredManifestFields)
	if err != nil {
		return q, err
	}
	m := Manifest{Schema: ManifestSchema, UID: q.UID, Root: q.Root, Containers: q.Containers, Entries: q.Entries}
	if q.Schema != SelectionSchema || len(q.Entries) == 0 || len(q.Entries) > PageSize {
		return q, fmt.Errorf("choose 1..32 entries for this batch")
	}
	return q, Validate(m)
}

func pageFailure(err error) Page {
	return Page{Schema: BrowseSchema, Status: "failed", Containers: []Entry{}, Entries: []Entry{}, Blocked: []BlockedEntry{}, Errors: []string{err.Error()}}
}

func BrowseFailure(err error) Page { return pageFailure(err) }
