package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

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
		build := buildversion.Current("sirsi").Version
		fabric := ledger.NewFabricGenerationProducerContext(ctx, collectDashboardFabricReadContext, build)
		defer fabric.Close()

		srv := dashboard.New(dashboard.Config{
			Port:       port,
			ListenHost: "0.0.0.0",
			CloudBoard: true,
			RouterFn:   collectDashboardRouter,
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

func init() {
	routerCmd.AddCommand(routerBoardServeCmd)
}
