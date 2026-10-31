package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
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
		var macsFn func() map[string]string
		if transport != nil && len(macs) == 0 {
			macsFn = watchTailnetMacs(ctx)
		}
		srv := dashboard.New(dashboard.Config{
			Port:          port,
			ListenHost:    "0.0.0.0",
			CloudBoard:    true,
			MacConsoles:   macs,
			MacConsolesFn: macsFn,
			MacTransport:  transport,
			RouterFn:      collectDashboardRouter,
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

// boardMacTag marks a Sirsi Mac on the tailnet. Every online peer carrying it
// is a console the board can drive; the tailnet policy decides which peers the
// board sees at all (tag:cloud-board reaches tag:sirsi-mac on 443 only).
const boardMacTag = "tag:sirsi-mac"

// tailnetMacConsoles reads `tailscale status --json`: each online peer tagged
// boardMacTag is a console at https://<its MagicDNS name>, named by its first
// DNS label (m1, m5, m5-studio).
func tailnetMacConsoles(status []byte) (map[string]string, error) {
	var st struct {
		Peer map[string]struct {
			DNSName string
			Online  bool
			Tags    []string
		}
	}
	if err := json.Unmarshal(status, &st); err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	macs := map[string]string{}
	for _, p := range st.Peer {
		host := strings.TrimSuffix(p.DNSName, ".")
		if !p.Online || host == "" || !slices.Contains(p.Tags, boardMacTag) {
			continue
		}
		name, _, _ := strings.Cut(host, ".")
		macs[name] = "https://" + host
	}
	return macs, nil
}

// watchTailnetMacs re-reads the tailnet every 30 s so a Mac that joins (or
// drops off) shows up on the board without a redeploy. A failed read keeps
// the last good list rather than emptying the board.
func watchTailnetMacs(ctx context.Context) func() map[string]string {
	var cur atomic.Pointer[map[string]string]
	cur.Store(&map[string]string{})
	refresh := func() {
		out, err := exec.CommandContext(ctx, "tailscale", "status", "--json").Output()
		if err == nil {
			var macs map[string]string
			if macs, err = tailnetMacConsoles(out); err == nil {
				cur.Store(&macs)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "board: tailnet discovery: %v\n", err)
	}
	refresh()
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				refresh()
			}
		}
	}()
	return func() map[string]string { return *cur.Load() }
}

// boardMacConsoles reads the Mac consoles the board drives. With
// SIRSI_BOARD_TS_PROXY (socks5://127.0.0.1:1055) set and no SIRSI_BOARD_MACS,
// the board discovers every tagged Sirsi Mac on the tailnet itself;
// SIRSI_BOARD_MACS ("m1=https://m1.<tailnet>.ts.net,…") pins a fixed list
// instead. No proxy means a read-only board.
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
	proxy := strings.TrimSpace(os.Getenv("SIRSI_BOARD_TS_PROXY"))
	if proxy == "" {
		if len(macs) > 0 {
			return nil, nil, fmt.Errorf("SIRSI_BOARD_MACS is set but SIRSI_BOARD_TS_PROXY is empty: Mac consoles are reachable only over the tailnet")
		}
		return nil, nil, nil
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
