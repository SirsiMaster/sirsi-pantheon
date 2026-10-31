package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// routerBoardServeCmd hosts the router surface (the same page and the same
// collectDashboardRouter producer as `sirsi dashboard`) for a cloud runtime.
// Every read resolves through routerstore, so pointed at the router service
// (SIRSI_ROUTER_URL) it renders the canonical ledger without running anything
// on a Mac. Only the read-only router routes are served (dashboard.CloudBoard).
var routerBoardServeCmd = &cobra.Command{
	Use:   "board-serve",
	Short: "Serve the read-only router board for a cloud runtime (Cloud Run)",
	Long: `Serves the router board — lanes, mailboxes, threads, drill-down — on 0.0.0.0:$PORT
(default 8080) from the canonical router store. Only the router surface and its
read-only APIs are reachable; workstation endpoints return 404. Sign-in is the
hosting platform's job (Cloud Run IAP), not this process's.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		port := 8080
		if v := os.Getenv("PORT"); v != "" {
			p, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("PORT %q: %w", v, err)
			}
			port = p
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := registerBoardThread(ctx); err != nil {
			return err
		}
		build := buildversion.Current("sirsi").Version
		fabric := ledger.NewFabricGenerationProducerContext(ctx, collectDashboardFabricReadContext, build)
		defer fabric.Close()

		macs, transport, err := boardMacConsoles()
		if err != nil {
			return err
		}
		srv := dashboard.New(dashboard.Config{
			Port:         port,
			ListenHost:   "0.0.0.0",
			CloudBoard:   true,
			MacConsoles:  macs,
			MacTransport: transport,
			RouterFn:     collectDashboardRouter,
			RoutabilityFn: func() (map[string]bool, error) {
				repoRoot, err := router.FindRepoRoot()
				if err != nil {
					return nil, err
				}
				return unroutableAgents(repoRoot)
			},
			FabricReadFn: collectDashboardFabricRead,
			FabricSource: fabric,
			FabricBuild:  build,
		})
		if err := srv.Start(); err != nil {
			return err
		}
		fmt.Printf("router board serving on 0.0.0.0:%d\n", port)
		<-ctx.Done()
		return srv.Stop()
	},
}

// boardMacConsoles reads the Mac consoles the board drives from
// SIRSI_BOARD_MACS ("M1=https://m1.<tailnet>.ts.net,M5=…") and reaches them
// through the board's Tailscale proxy, SIRSI_BOARD_TS_PROXY
// (socks5://127.0.0.1:1055). No consoles configured means a read-only board.
func boardMacConsoles() (map[string]string, http.RoundTripper, error) {
	macs := map[string]string{}
	for _, pair := range strings.Split(os.Getenv("SIRSI_BOARD_MACS"), ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		name, base, ok := strings.Cut(pair, "=")
		u, err := url.Parse(strings.TrimSpace(base))
		if !ok || strings.TrimSpace(name) == "" || err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, nil, fmt.Errorf("SIRSI_BOARD_MACS entry %q: want name=https://host", pair)
		}
		macs[strings.TrimSpace(name)] = strings.TrimRight(u.String(), "/")
	}
	if len(macs) == 0 {
		return nil, nil, nil
	}
	proxy := strings.TrimSpace(os.Getenv("SIRSI_BOARD_TS_PROXY"))
	if proxy == "" {
		return nil, nil, fmt.Errorf("SIRSI_BOARD_MACS is set but SIRSI_BOARD_TS_PROXY is empty: Mac consoles are reachable only over the tailnet")
	}
	pu, err := url.Parse(proxy)
	if err != nil || pu.Scheme != "socks5" {
		return nil, nil, fmt.Errorf("SIRSI_BOARD_TS_PROXY %q: want socks5://host:port", proxy)
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyURL(pu)
	return macs, t, nil
}

// registerBoardThread gives the board its own registered thread (Rule of Ra:
// the whole-ledger reads are not exempt) and keeps it alive every minute,
// re-registering if it was reaped while the service was scaled to zero. The
// thread id travels in SIRSI_THREAD_ID, which every routerstore.Resolve reads.
func registerBoardThread(ctx context.Context) error {
	repo, err := router.FindRepoRoot()
	if err != nil {
		return fmt.Errorf("board: locate registry: %w", err)
	}
	root := filepath.Join(repo, ".agents", "idea-router")
	agent := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID"))
	if agent == "" {
		agent = "router-board"
		_ = os.Setenv("SIRSI_AGENT_ID", agent)
	}
	register := func() error {
		t, err := router.RegisterThread(root, &router.Thread{AgentID: agent, Surface: "service"})
		if err != nil {
			return fmt.Errorf("board: register %s thread: %w", agent, err)
		}
		return os.Setenv("SIRSI_THREAD_ID", t.ThreadID)
	}
	if err := register(); err != nil {
		return err
	}
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				_, _ = router.CloseThread(root, os.Getenv("SIRSI_THREAD_ID"))
				return
			case <-tick.C:
			}
			_, err := router.Heartbeat(root, os.Getenv("SIRSI_THREAD_ID"), router.HeartbeatUpdate{})
			if errors.Is(err, router.ErrHeartbeatThreadUnavailable) {
				err = register()
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "board: heartbeat: %v\n", err)
			}
		}
	}()
	return nil
}

func init() {
	routerCmd.AddCommand(routerBoardServeCmd)
}
