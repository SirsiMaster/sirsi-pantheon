//go:build darwin

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/spf13/cobra"
)

type maatPodServiceOptions struct {
	pod, journal, listen, cert, key, clientCA, recipeDir string
	clients                                              []string
}

func newMaatPodServiceCmd() *cobra.Command {
	var opts maatPodServiceOptions
	cmd := &cobra.Command{
		Use:   "authority-service",
		Short: "Plan or install a dormant native launchd recipe for a Ma'at pod authority",
		Args:  cobra.NoArgs,
		Long: `This prepares a signed, installed sirsi engine recipe for explicit user activation.
Neither subcommand loads launchd or starts the authority; login startup requires a
separate explicit opt-in. Runtime renewal, reconciliation, and ongoing self-fencing
are not claimed by preparation and need separate qualification. An existing host
router-service enrollment is required; a cut-over spool marker is recovered by
routerstore.Resolve. Credentials are never copied into the launchd recipe.`,
	}
	planCmd := &cobra.Command{
		Use:   "plan",
		Short: "Validate current canonical authority inputs and print a dry plan",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := resolveMaatPodServicePlan(opts)
			if err != nil {
				return err
			}
			return writeMaatPodServicePlan(cmd, plan)
		},
	}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Create the private dormant recipe without activating it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			plan, err := resolveMaatPodServicePlan(opts)
			if err != nil {
				return err
			}
			plan, err = setup.InstallMaatPodServiceRecipe(plan)
			if err != nil {
				return err
			}
			return writeMaatPodServicePlan(cmd, plan)
		},
	}
	flags := cmd.PersistentFlags()
	flags.StringVar(&opts.pod, "pod", "", "Existing canonical pod delegation")
	flags.StringVar(&opts.journal, "journal", "", "Existing canonical absolute pod journal path (not created during preparation)")
	flags.StringVar(&opts.listen, "listen", "", "Literal private/Tailscale/LAN address and port from the delegation")
	flags.StringVar(&opts.cert, "cert", "", "Existing approved service certificate path")
	flags.StringVar(&opts.key, "key", "", "Existing approved 0600 private key path")
	flags.StringVar(&opts.clientCA, "client-ca", "", "Existing approved client CA bundle path")
	flags.StringArrayVar(&opts.clients, "client", nil, "Approved SHA256=registered-agent binding (repeatable)")
	flags.StringVar(&opts.recipeDir, "recipe-dir", "", "Existing owner-private directory (0700) for the dormant recipe")
	cmd.AddCommand(planCmd, installCmd)
	return cmd
}

func resolveMaatPodServicePlan(opts maatPodServiceOptions) (setup.MaatPodServicePlan, error) {
	if opts.pod == "" || opts.journal == "" || opts.listen == "" || opts.cert == "" || opts.key == "" || opts.clientCA == "" || opts.recipeDir == "" || len(opts.clients) == 0 {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: pod, journal, listen, existing TLS selectors, recipe directory, and client bindings are required")
	}
	// Resolve can create a deliberately selected SQLite sandbox. Refuse that
	// selector before resolving anything: a dry plan must not create a store.
	if strings.TrimSpace(os.Getenv("SIRSI_ROUTER_DB")) != "" {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: local SQLite sandbox selectors are not a production authority route")
	}
	machine := router.MachineID()
	if machine == "" {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: native machine identity is unavailable")
	}
	store, err := routerstore.Resolve()
	if err != nil {
		return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: resolve canonical ledger: %w", err)
	}
	defer store.Close()
	if _, ok := store.(*routerstore.RemoteStore); !ok {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: production packaging requires the canonical router service; local SQLite ledgers are test/sandbox stores")
	}
	canonical, ok := store.(schedule.AtomicDelegationScheduler)
	if !ok {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: canonical ledger lacks atomic delegation reads")
	}
	cache, err := schedule.FetchFiniteGrantCache(canonical, opts.pod)
	if err != nil {
		return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: fresh canonical delegation unavailable: %w", err)
	}
	delegation, err := cache.Snapshot()
	if err != nil {
		return setup.MaatPodServicePlan{}, err
	}
	now := time.Now()
	if err := setup.ValidateMaatPodDelegation(*delegation, opts.pod, machine, now); err != nil {
		return setup.MaatPodServicePlan{}, err
	}
	root, err := router.FindRepoRoot()
	if err != nil {
		return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: canonical registry working directory unavailable: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return setup.MaatPodServicePlan{}, errors.New("maat pod service: canonical registry working directory is invalid")
	}
	registry, err := router.LoadRegistry(filepath.Join(root, ".agents", "idea-router"))
	if err != nil {
		return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: load canonical registered-agent registry: %w", err)
	}
	for _, binding := range opts.clients {
		_, agent, ok := strings.Cut(binding, "=")
		if !ok || agent == "" {
			return setup.MaatPodServicePlan{}, errors.New("maat pod service: client bindings must be SHA256=registered-agent")
		}
		if _, exists := registry.Agents[agent]; !exists {
			return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: client agent %q is not registered in the canonical registry", agent)
		}
	}
	executable, err := setup.CurrentPackagedMaatEngine()
	if err != nil {
		return setup.MaatPodServicePlan{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return setup.MaatPodServicePlan{}, fmt.Errorf("maat pod service: user home unavailable: %w", err)
	}
	return setup.BuildMaatPodServicePlan(setup.MaatPodServiceSpec{
		Pod: opts.pod, JournalPath: opts.journal, Listen: opts.listen,
		CertificatePath: opts.cert, PrivateKeyPath: opts.key, ClientCAPath: opts.clientCA,
		ClientBindings: opts.clients, WorkingDirectory: root, RecipeDirectory: opts.recipeDir,
		HomeDirectory: home, Executable: executable, MachineID: machine,
		Delegation: *delegation, Now: now,
	}, setup.VerifyPackagedMaatEngine)
}

func writeMaatPodServicePlan(cmd *cobra.Command, plan setup.MaatPodServicePlan) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
		Status                  string   `json:"status"`
		GlobalState             string   `json:"global_state"`
		ActionsPerformed        []string `json:"actions_performed"`
		EngineSignatureVerified bool     `json:"engine_signature_verified"`
		Recipe                  string   `json:"recipe"`
		ProgramArguments        []string `json:"program_arguments"`
		StartArgv               []string `json:"start_argv"`
		LoginOptInArgv          []string `json:"login_opt_in_argv"`
		RuntimeQualification    string   `json:"runtime_qualification"`
	}{
		Status: plan.Status, GlobalState: plan.GlobalState, ActionsPerformed: plan.ActionsPerformed,
		EngineSignatureVerified: plan.EngineSignatureVerified, Recipe: plan.PlistPath,
		ProgramArguments: plan.ProgramArguments, StartArgv: plan.StartArgv,
		LoginOptInArgv: plan.LoginOptInArgv, RuntimeQualification: plan.RuntimeQualification,
	})
}

func init() { maatCmd.AddCommand(newMaatPodServiceCmd()) }
