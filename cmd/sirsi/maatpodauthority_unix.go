//go:build darwin || linux

package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/spf13/cobra"
)

func newMaatPodAuthorityCmd() *cobra.Command {
	var pod, journal, address, certPath, keyPath, caPath, grantCache string
	var identities []string
	c := &cobra.Command{Use: "authority-serve", Short: "Serve an existing finite native delegation with approved mutual TLS identities"}
	c.Flags().StringVar(&grantCache, "grant-cache", "", "Immutable signed finite delegation cache path for offline restart")
	c.Flags().StringVar(&pod, "pod", "", "Existing canonical pod delegation")
	c.Flags().StringVar(&journal, "journal", "", "Private canonical absolute event journal path")
	c.Flags().StringVar(&address, "listen", "127.0.0.1:9443", "Explicit native listener address")
	c.Flags().StringVar(&certPath, "cert", "", "Existing approved service certificate path")
	c.Flags().StringVar(&keyPath, "key", "", "Existing approved service private-key path; never copied or printed")
	c.Flags().StringVar(&caPath, "client-ca", "", "Existing approved native client CA bundle path")
	c.Flags().StringSliceVar(&identities, "client", nil, "Explicit approved certificate SHA256=registered-agent binding (repeatable)")
	c.RunE = func(cmd *cobra.Command, _ []string) error {
		if pod == "" || journal == "" || certPath == "" || keyPath == "" || caPath == "" || len(identities) == 0 {
			return fmt.Errorf("maat: existing delegation, journal and approved native TLS paths/bindings required")
		}
		st, err := routerstore.Resolve()
		if err != nil {
			return err
		}
		defer st.Close()
		canonical, ok := st.(schedule.AtomicDelegationScheduler)
		if !ok {
			return fmt.Errorf("maat: compatible canonical delegation API required")
		}
		root, err := router.FindRepoRoot()
		if err != nil {
			return err
		}
		reg, err := router.LoadRegistry(root + "/.agents/idea-router")
		if err != nil {
			return err
		}
		holders := map[string]string{}
		for _, binding := range identities {
			hash, agent, ok := strings.Cut(binding, "=")
			if !ok {
				return fmt.Errorf("maat: invalid client binding")
			}
			if _, ok = reg.Agents[agent]; !ok {
				return fmt.Errorf("maat: client agent is not registered")
			}
			if _, ok = holders[hash]; ok {
				return fmt.Errorf("maat: duplicate client fingerprint")
			}
			holders[hash] = agent
		}
		pair, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return fmt.Errorf("maat: approved service identity could not be loaded")
		}
		cache, err := schedule.FetchFiniteGrantCache(canonical, pod)
		if errors.Is(err, schedule.ErrGrantLookupUnavailable) && grantCache != "" {
			cache, err = schedule.LoadFiniteGrantCache(grantCache, pair)
		}
		if err != nil {
			return err
		}
		if grantCache != "" {
			if err = schedule.PersistFiniteGrantCache(grantCache, cache, pair); err != nil {
				return err
			}
		}
		d, err := cache.Snapshot()
		if err != nil {
			return err
		}
		if d.Pod != pod || d.AuthorityMachine != router.MachineID() {
			return fmt.Errorf("maat: this actual native machine is not delegated")
		}
		ca, err := os.ReadFile(caPath)
		if err != nil {
			return fmt.Errorf("maat: approved client CA unavailable")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return fmt.Errorf("maat: invalid approved CA bundle")
		}
		j, err := schedule.OpenPodJournal(journal, router.MachineID(), *d)
		if err != nil {
			return err
		}
		defer j.Close()
		handler, err := schedule.PodHTTPHandler(j, holders)
		if err != nil {
			return err
		}
		server, err := schedule.NativePodServer(address, &tls.Config{Certificates: []tls.Certificate{pair}, ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert}, handler)
		if err != nil {
			return err
		}
		go func() { <-cmd.Context().Done(); server.Close() }()
		err = server.ListenAndServeTLS("", "")
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
	return c
}
func init() { maatCmd.AddCommand(newMaatPodAuthorityCmd()) }
