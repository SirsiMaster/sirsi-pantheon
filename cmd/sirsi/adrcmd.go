package main

// `sirsi adr` — durable ADR number allocation.
//
// ADR numbers used to be chosen by reading the filesystem and counting. That
// races: two agents branching from the same main both see the same highest
// number and both claim it. origin/main carries two distinct ADR-054 documents
// because of exactly that. Allocation now goes through the router store, which
// already serializes item and task creation for the same reason.
//
// Workflow:
//
//	sirsi adr claim --title "..." --agent claude-home   -> ADR-058 (reserved)
//	sirsi adr publish 58 --slug docs/ADR-058-FOO.md     -> marks it published
//	sirsi adr list                                      -> the allocation table
//	sirsi adr audit                                     -> docs/ vs the store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

// ADR filenames are either a base document (ADR-031-LOCAL-MODELS.md) or a
// lettered SUB-PART that amends it (ADR-031-A-NEVER-EXHAUST-THE-HOST.md).
// Sub-parts are a real, load-bearing convention here — ADR-031-A/B/C each
// amend ADR-031 — so they share the base number legitimately and must NOT be
// reported as collisions. Treating them as collisions was a false positive
// that would have made the gate cry wolf on 4 of its 5 findings.
var adrFilePattern = regexp.MustCompile(`^ADR-(\d{3,})(-[A-Z])?-(.+)\.md$`)

func newADRCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "adr",
		Short: "Allocate and audit ADR numbers through the durable store",
		Long: "ADR numbers are allocated by the router store, not by counting files.\n" +
			"Two agents can never be handed the same number: allocation is a single\n" +
			"serialized transaction and the store holds PRIMARY KEY (namespace, number).",
	}
	cmd.AddCommand(newADRClaimCmd(), newADRPublishCmd(), newADRListCmd(), newADRAuditCmd())
	return cmd
}

func newADRClaimCmd() *cobra.Command {
	var title, agent string
	cmd := &cobra.Command{
		Use:   "claim",
		Short: "Reserve the next ADR number",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			id, err := st.AllocateIdentifier("ADR", title, agent)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s reserved for %s\n  title: %s\n  next: write docs/%s-<SLUG>.md, then `sirsi adr publish %d --slug <path>`\n",
				id.Name(), id.Owner, id.Title, id.Name(), id.Number)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "ADR title (required)")
	cmd.Flags().StringVar(&agent, "agent", "", "allocating agent id (required)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func newADRPublishCmd() *cobra.Command {
	var slug string
	cmd := &cobra.Command{
		Use:   "publish <number>",
		Short: "Mark an allocated ADR as published at a path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid ADR number %q", args[0])
			}
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			if err := st.PublishIdentifier("ADR", n, slug); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ADR-%03d published at %s\n", n, slug)
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "path to the ADR document (required)")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func newADRListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every allocated ADR number and its owner",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()
			ids, err := st.ListIdentifiers("ADR")
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no ADR numbers allocated yet — run `sirsi adr audit --backfill` to import docs/")
				return nil
			}
			w := cmd.OutOrStdout()
			for _, id := range ids {
				fmt.Fprintf(w, "%-9s %-11s %-18s %s\n", id.Name(), id.Status, id.Owner, id.Title)
			}
			return nil
		},
	}
}

// newADRAuditCmd reconciles docs/ against the allocation table. This is the
// gate: a document whose number was never allocated, or two documents sharing
// a number, is reported as a hard failure.
func newADRAuditCmd() *cobra.Command {
	var backfill bool
	var docsDir string
	var grandfathered []int
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Reconcile docs/ADR-*.md against the allocation table",
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openRouterStoreForADR()
			if err != nil {
				return err
			}
			defer st.Close()

			byNumber, subParts, err := scanADRDocs(docsDir)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()

			// Ratchet, not a big-bang cleanup. Four collisions already exist on
			// main (013, 016, 046, 054); a gate that fails the build on day one
			// for pre-existing debt gets switched off, and then it protects
			// nothing. Known numbers are reported loudly but do not fail; any
			// NEW collision does. The allow-list is the debt made visible, and
			// it is only ever allowed to shrink.
			allowed := map[int]bool{}
			for _, n := range grandfathered {
				allowed[n] = true
			}
			var collisions, knownCollisions, unallocated int
			numbers := make([]int, 0, len(byNumber))
			for n := range byNumber {
				numbers = append(numbers, n)
			}
			sort.Ints(numbers)

			for _, n := range numbers {
				files := byNumber[n]
				if len(files) > 1 {
					label := "COLLISION"
					if allowed[n] {
						label = "KNOWN COLLISION (grandfathered — must be renumbered)"
						knownCollisions++
					} else {
						collisions++
					}
					fmt.Fprintf(w, "%s ADR-%03d claimed by %d documents:\n", label, n, len(files))
					for _, f := range files {
						fmt.Fprintf(w, "    %s\n", f)
					}
					continue
				}
				if backfill {
					if _, err := st.ClaimIdentifierNumber("ADR", n, files[0], filepath.Join(docsDir, files[0]), "backfill"); err != nil {
						fmt.Fprintf(w, "UNCLAIMABLE ADR-%03d (%s): %v\n", n, files[0], err)
						unallocated++
					}
				}
			}

			var subCount int
			for _, f := range subParts {
				subCount += len(f)
			}
			fmt.Fprintf(w, "\n%d base ADR documents, %d lettered sub-parts, %d new collisions, %d grandfathered\n",
				len(numbers), subCount, collisions, knownCollisions)
			// A grandfathered number that is no longer colliding means the debt
			// was paid — tell the operator to tighten the ratchet.
			for _, n := range grandfathered {
				if len(byNumber[n]) <= 1 {
					fmt.Fprintf(w, "RESOLVED ADR-%03d no longer collides — remove it from --allow to lock the fix in\n", n)
				}
			}
			if collisions > 0 {
				return fmt.Errorf("%d ADR number collision(s) — every citation of a collided number is ambiguous; renumber the later document via `sirsi adr claim`", collisions)
			}
			if unallocated > 0 {
				return fmt.Errorf("%d ADR document(s) could not be claimed", unallocated)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&backfill, "backfill", false, "claim existing docs/ADR-*.md numbers into the store")
	cmd.Flags().IntSliceVar(&grandfathered, "allow", nil, "ADR numbers with pre-existing collisions: reported, not failed (the debt list — only ever shrink it)")
	cmd.Flags().StringVar(&docsDir, "docs", "docs", "directory holding ADR documents")
	return cmd
}

// scanADRDocs maps ADR number -> filenames claiming it. More than one filename
// for a number is the collision this whole subsystem exists to end.
func scanADRDocs(dir string) (map[int][]string, map[int][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", dir, err)
	}
	out := map[int][]string{}
	subParts := map[int][]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := adrFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if m[2] != "" {
			// A lettered sub-part legitimately shares its base number.
			subParts[n] = append(subParts[n], e.Name())
			continue
		}
		out[n] = append(out[n], e.Name())
	}
	for n := range out {
		sort.Strings(out[n])
	}
	for n := range subParts {
		sort.Strings(subParts[n])
	}
	return out, subParts, nil
}

func openRouterStoreForADR() (routerstore.Store, error) {
	// Same resolution as dispatch.Open: ~/.sirsi/router.db, with
	// SIRSI_ROUTER_DB overriding it. Matching that exactly matters — an ADR
	// allocator pointed at a different database than the router would hand out
	// numbers nobody else can see, which is the drift it exists to prevent.
	return routerstore.Resolve()
}
