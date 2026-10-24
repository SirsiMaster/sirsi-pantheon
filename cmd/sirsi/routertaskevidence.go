//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// Walk every component without following symlinks. O_NONBLOCK prevents a FIFO
// leaf from hanging before the regular-file check. No contents are executed.
func openTaskEvidenceFile(path string) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("task evidence: canonical absolute input path required")
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(fd, part, flags, 0)
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, openErr
		}
		fd = next
	}
	f := os.NewFile(uintptr(fd), path)
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("task evidence: input must be a regular file: %v", err)
	}
	return f, nil
}

func copyTaskEvidenceFile(path string, limit int64, dst io.Writer) error {
	f, err := openTaskEvidenceFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return err
	}
	n, err := io.Copy(dst, io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("task evidence: file exceeds %d byte bound", limit)
	}
	after, err := f.Stat()
	if err != nil {
		return err
	}
	named, err := openTaskEvidenceFile(path)
	if err != nil {
		return err
	}
	defer named.Close()
	info, err := named.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, after) || !os.SameFile(before, info) || before.Size() != after.Size() || before.Size() != info.Size() || !before.ModTime().Equal(after.ModTime()) || !before.ModTime().Equal(info.ModTime()) || n != after.Size() {
		return errors.New("task evidence: file changed while reading")
	}
	return nil // Point-in-time custody, not a lease on future client file bytes.
}

type taskEvidenceMapJSONSchema uint8

const (
	taskEvidenceMapAncillary taskEvidenceMapJSONSchema = iota
	taskEvidenceMapRoot
	taskEvidenceMapRows
	taskEvidenceMapRow
)

// Ancillary preservation metadata stays allowed, but no object may have
// duplicate keys and no recognized control key may use a case alias. Aliases
// are rejected, never folded into an accepted spelling.
func validateTaskEvidenceMapJSON(b []byte) error {
	if len(b) > 4<<20 || !utf8.Valid(b) {
		return errors.New("task evidence: invalid/bounded preservation map required")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := taskEvidenceMapJSONValue(d, taskEvidenceMapRoot, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("task evidence: trailing preservation map JSON")
	}
	return nil
}

func taskEvidenceMapJSONValue(d *json.Decoder, schema taskEvidenceMapJSONSchema, depth int) error {
	if depth > 16 {
		return errors.New("task evidence: preservation map nesting exceeds bound")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, composite := t.(json.Delim)
	if (schema == taskEvidenceMapRoot || schema == taskEvidenceMapRow) && (!composite || delim != '{') {
		return errors.New("task evidence: preservation map object required")
	}
	if schema == taskEvidenceMapRows && (!composite || delim != '[') {
		return errors.New("task evidence: preservation map rows array required")
	}
	if !composite {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		child := taskEvidenceMapAncillary
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return errors.New("task evidence: duplicate preservation map key")
			}
			seen[name] = true
			var recognized []string
			switch schema {
			case taskEvidenceMapRoot:
				recognized = []string{"schema", "rows"}
				if name == "rows" {
					child = taskEvidenceMapRows
				}
			case taskEvidenceMapRow:
				recognized = []string{"old_absolute", "preserved_absolute", "sha256", "type"}
			}
			for _, exact := range recognized {
				if name != exact && strings.EqualFold(name, exact) {
					return errors.New("task evidence: exact-case preservation map key required")
				}
			}
		} else if schema == taskEvidenceMapRows {
			child = taskEvidenceMapRow
		}
		if err := taskEvidenceMapJSONValue(d, child, depth+1); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil {
		return err
	}
	close := json.Delim('}')
	if delim == '[' {
		close = ']'
	}
	if end != close {
		return errors.New("task evidence: malformed preservation map JSON")
	}
	return nil
}

func loadTaskEvidencePlan(manifestPath, mapPath string) (routerstore.TaskEvidenceRequest, error) {
	var request routerstore.TaskEvidenceRequest
	var manifest, preserved bytes.Buffer
	if err := copyTaskEvidenceFile(manifestPath, routerstore.TaskEvidenceManifestLimit, &manifest); err != nil {
		return request, err
	}
	// Reuse the strict RPC decoder for the same closed nested schema locally.
	wrapped := append([]byte(`{"action":"inspect","manifest":`), manifest.Bytes()...)
	wrapped = append(wrapped, '}')
	if err := json.Unmarshal(wrapped, &request); err != nil {
		return request, err
	}
	if err := routerstore.ValidateTaskEvidenceRequest(request); err != nil {
		return request, err
	}
	if err := copyTaskEvidenceFile(mapPath, 4<<20, &preserved); err != nil {
		return request, err
	}
	sum := sha256.Sum256(preserved.Bytes())
	if hex.EncodeToString(sum[:]) != request.Manifest.PreservationMapSHA256 {
		return request, errors.New("task evidence: preservation-map digest mismatch")
	}
	if err := validateTaskEvidenceMapJSON(preserved.Bytes()); err != nil {
		return request, err
	}
	var preservation struct {
		Schema string `json:"schema"`
		Rows   []struct {
			Old    string `json:"old_absolute"`
			New    string `json:"preserved_absolute"`
			SHA256 string `json:"sha256"`
			Type   string `json:"type"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(preserved.Bytes(), &preservation); err != nil {
		return request, err
	}
	if preservation.Schema != "pantheon.checkout-preserved-namespace.v1" || len(preservation.Rows) < 1 || len(preservation.Rows) > 4096 {
		return request, errors.New("task evidence: bounded preservation map required")
	}
	for _, row := range request.Manifest.Tasks {
		for _, change := range row.Changes {
			old, _ := routerstore.TaskEvidenceLocalPath(change.OldURI)
			newPath, _ := routerstore.TaskEvidenceLocalPath(change.NewURI)
			matches := 0
			for _, entry := range preservation.Rows {
				if entry.Old == old {
					matches++
					if entry.New != newPath || entry.SHA256 != change.SHA256 || entry.Type != "regular" {
						return request, errors.New("task evidence: preservation mapping differs from manifest")
					}
				}
			}
			if matches != 1 {
				return request, errors.New("task evidence: missing/ambiguous preservation mapping")
			}
			h := sha256.New()
			if err := copyTaskEvidenceFile(newPath, 64<<20, h); err != nil {
				return request, err
			}
			if hex.EncodeToString(h.Sum(nil)) != change.SHA256 {
				return request, errors.New("task evidence: replacement artifact digest mismatch")
			}
			// The old file may be intentionally absent after preservation. Never
			// recreate it or require a symlink/checkout restoration to plan.
		}
	}
	return request, nil
}

func callTaskEvidenceAuthority(ctx context.Context, req routerstore.TaskEvidenceRequest) (routerstore.TaskEvidenceResult, error) {
	// Do not let Resolve create an explicit sandbox DB before discovering that
	// it is the wrong authority. Normal envless enrollment recovery stays intact.
	if strings.TrimSpace(os.Getenv("SIRSI_ROUTER_URL")) == "" && strings.TrimSpace(os.Getenv("SIRSI_ROUTER_DB")) != "" {
		return routerstore.TaskEvidenceResult{}, errors.New("task evidence: inspection/apply requires configured canonical service/relay, not a local DB")
	}
	st, err := routerstore.Resolve()
	if err != nil {
		return routerstore.TaskEvidenceResult{}, err
	}
	defer st.Close()
	if _, ok := st.(*routerstore.RemoteStore); !ok {
		return routerstore.TaskEvidenceResult{}, errors.New("task evidence: canonical service/relay required")
	}
	return st.RelocateTaskEvidence(ctx, req)
}

func newTaskEvidenceRelocationCmd(call func(context.Context, routerstore.TaskEvidenceRequest) (routerstore.TaskEvidenceResult, error)) *cobra.Command {
	var manifestPath, mapPath string
	var inspect, apply bool
	cmd := &cobra.Command{
		Use: "relocate-evidence", Args: cobra.NoArgs,
		Short: "Plan or apply a finite own-task evidence URI relocation without lifecycle changes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if inspect && apply {
				return errors.New("choose --inspect or --apply, not both")
			}
			req, err := loadTaskEvidencePlan(manifestPath, mapPath)
			if err != nil {
				return err
			}
			if !inspect && !apply {
				b, _ := json.Marshal(req.Manifest)
				sum := sha256.Sum256(b)
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Action         string `json:"action"`
					ManifestSHA256 string `json:"manifest_sha256"`
					Qualification  string `json:"qualification"`
				}{"local-plan", hex.EncodeToString(sum[:]), "local map/replacement bytes checked only; no authority contacted, row CAS or external review verified"})
			}
			if apply {
				req.Action = "apply"
			}
			if err := routerstore.ValidateTaskEvidenceRequest(req); err != nil {
				return err
			}
			out, err := call(cmd.Context(), req)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(out)
		},
	}
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Absolute closed v1 manifest JSON (no credentials)")
	cmd.Flags().StringVar(&mapPath, "preservation-map", "", "Absolute preservation map whose exact digest the manifest binds")
	cmd.Flags().BoolVar(&inspect, "inspect", false, "Read current own-row fingerprints through configured canonical authority")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply the exact reviewed manifest through configured canonical authority")
	return cmd
}

func init() { routerTaskCmd.AddCommand(newTaskEvidenceRelocationCmd(callTaskEvidenceAuthority)) }
