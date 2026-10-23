//go:build darwin || linux

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

func decodeMailboxRequest(r io.Reader) (routerstore.MailboxReconcileRequest, string, error) {
	var req routerstore.MailboxReconcileRequest
	b, err := io.ReadAll(io.LimitReader(r, (128<<10)+1))
	if err != nil || len(b) > 128<<10 {
		return req, "", fmt.Errorf("mailbox request is unreadable or exceeds 128 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&req); err != nil {
		return req, "", err
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return req, "", fmt.Errorf("mailbox request has trailing content")
	}
	h := sha256.Sum256(b)
	return req, hex.EncodeToString(h[:]), nil
}

func newMailboxReconcileCmd() *cobra.Command {
	var path string
	var apply bool
	cmd := &cobra.Command{
		Use: "reconcile-notice", Args: cobra.NoArgs,
		Short: "Plan or apply an exact finite, operator-approved service-notice acknowledgment",
		Long:  "Default is a read-only request digest. --apply calls the canonical service/relay. Approval requires its existing bootstrap operator; a lane can only acknowledge the immutable manifest's exact notices as itself. No owner dismissal, generic foreign mailbox access, alias, consumer, or product completion is granted.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !filepath.IsAbs(path) {
				return fmt.Errorf("an absolute --request JSON path is required")
			}
			fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				return err
			}
			f := os.NewFile(uintptr(fd), path)
			defer func() { _ = f.Close() }()
			info, err := f.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("mailbox request must be a regular non-symlink file")
			}
			req, digest, err := decodeMailboxRequest(f)
			if err != nil {
				return err
			}
			if !apply {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
					Action, RequestSHA256 string
					Apply                 bool
				}{req.Action, digest, false})
			}
			st, err := routerstore.Resolve()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			if _, ok := st.(*routerstore.RemoteStore); !ok {
				return fmt.Errorf("mailbox reconciliation requires the canonical service or its relay, never a local ledger")
			}
			result, err := st.ReconcileMailboxNotice(cmd.Context(), req)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}
	cmd.Flags().StringVar(&path, "request", "", "Absolute bounded request JSON file (no credentials)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply through the authenticated canonical service")
	return cmd
}

func newMailboxFingerprintCmd() *cobra.Command {
	return &cobra.Command{
		Use: "notice-fingerprint <id>", Args: cobra.ExactArgs(1),
		Short: "Read one permitted canonical service notice and print its exact reconciliation digest",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := routerstore.Resolve()
			if err != nil {
				return err
			}
			defer func() { _ = st.Close() }()
			if _, ok := st.(*routerstore.RemoteStore); !ok {
				return fmt.Errorf("notice capture requires the canonical service or relay")
			}
			it, err := st.Get(args[0]) // Existing read rights; never a foreign-read exception.
			if err != nil {
				return err
			}
			if it.To != "horus" || it.Status != routerstore.StatusOpen || it.Type != "decision" {
				return fmt.Errorf("not an open Horus coordination decision")
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(routerstore.MailboxNotice{ID: it.ID, ExpectedSHA256: routerstore.MailboxNoticeSHA256(it)})
		},
	}
}

func init() { routerCmd.AddCommand(newMailboxReconcileCmd(), newMailboxFingerprintCmd()) }
