package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
)

var (
	boardServePort  int
	boardServeDir   string
	boardServePoll  time.Duration
	boardServeOnce  bool
	boardServeShape string
)

var boardServeCmd = &cobra.Command{
	Use:   "board-serve",
	Short: "Serve the router board and ledger (Go; replaces the out-of-repo Python server)",
	Long: `Serves the router board and ledger on 127.0.0.1:8734.

This is the Go port of ~/.sirsi/ledger-dashboard/server.py, which lived outside
the repo and was therefore never reviewed, tested, or shipped with the CLI it
reported on. Owner directive 2026-08-06: everything on this machine is Go unless
Python is required to operate or genuinely better. Nothing here required Python.

The UI (index.html) is unchanged and served as a static asset — it is a page,
not a service, and rewriting it would risk the surface the owner actually reads.
It is served from disk, not go:embed, so it can be hand-edited without a
rebuild — but the canonical, version-controlled copy lives at
internal/routerboard/ui/index.html. That copy was previously untracked
(2026-08-07 incident: the only copy lived solely at ~/.sirsi/ledger-dashboard,
outside git, with no review trail and no recovery if the disk copy was lost).
After editing ~/.sirsi/ledger-dashboard/index.html, copy the change back into
internal/routerboard/ui/index.html and commit it — the disk copy is the
runtime, the repo copy is the record.

  sirsi board-serve
  sirsi board-serve --port 8734 --poll 3s`,
	RunE: runRouterBoard,
}

func init() {
	boardServeCmd.Flags().IntVar(&boardServePort, "port", 8734, "Port to serve on")
	boardServeCmd.Flags().StringVar(&boardServeDir, "dir", "", "Directory holding index.html (default ~/.sirsi/ledger-dashboard)")
	boardServeCmd.Flags().DurationVar(&boardServePoll, "poll", 3*time.Second, "Router poll interval")
	boardServeCmd.Flags().BoolVar(&boardServeOnce, "once", false, "Poll once, print the payload as JSON, exit")
	boardServeCmd.Flags().StringVar(&boardServeShape, "shape", "board", "Output shape for --once: board|fleet (fleet is the menubar projection)")
	rootCmd.AddCommand(boardServeCmd)
}

func runRouterBoard(_ *cobra.Command, _ []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	dir := boardServeDir
	if dir == "" {
		dir = filepath.Join(home, ".sirsi", "ledger-dashboard")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "index.html")); statErr != nil {
		return fmt.Errorf("board UI not found at %s/index.html: %w", dir, statErr)
	}

	// Absolute path to the binary: under launchd, PATH lacks ~/.local/bin, and
	// a bare "sirsi" failed silently — the board rendered ZEROS as if that were
	// data. Prefer this executable so the board and the CLI it reports on can
	// never be different builds.
	bin, err := os.Executable()
	if err != nil || bin == "" {
		bin = filepath.Join(home, ".local", "bin", "sirsi")
	}

	repoRoot, rerr := findRepoRootForBoard()
	if rerr != nil {
		return fmt.Errorf("locate repo root for agents.json: %w", rerr)
	}
	agentsJSON := filepath.Join(repoRoot, ".agents", "idea-router", "agents.json")

	b := routerboard.New(bin, agentsJSON, routerboard.BuildID(dir))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --once exists so surfaces that shell out (the menubar) render the SAME
	// numbers as the served board, computed by the SAME code. A second
	// aggregation, however careful, is a second producer — and two producers
	// disagree by construction, which is the defect this whole port removes.
	if boardServeOnce {
		b.Poll(ctx)
		var (
			body    []byte
			version uint64
			err     error
		)
		if boardServeShape == "fleet" {
			body, version, err = b.SnapshotFleetShape()
			if err != nil {
				return fmt.Errorf("board: project fleet shape: %w", err)
			}
		} else {
			body, version = b.Snapshot()
		}
		if version == 0 || len(body) == 0 {
			return fmt.Errorf("board: no poll completed — refusing to print an empty payload")
		}
		fmt.Println(string(body))
		return nil
	}
	go b.Run(ctx, boardServePoll)

	mux := http.NewServeMux()
	routerboard.NewHandler(b, dir).Register(mux)
	srv := &http.Server{
		Addr:        fmt.Sprintf("127.0.0.1:%d", boardServePort),
		Handler:     mux,
		ReadTimeout: 10 * time.Second,
		// No write timeout: /api/stream is a long-lived SSE connection and a
		// deadline would sever the board mid-watch.
		WriteTimeout: 0,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "board-serve: %v\n", err)
			cancel()
		}
	}()
	fmt.Printf("Router board serving at http://127.0.0.1:%d (build %s)\n", boardServePort, routerboard.BuildID(dir))

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	shutdown, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	return srv.Shutdown(shutdown)
}

// findRepoRootForBoard resolves the repo whose .agents/idea-router/agents.json
// carries wake and type metadata for each lane.
func findRepoRootForBoard() (string, error) {
	return router.FindRepoRoot()
}
