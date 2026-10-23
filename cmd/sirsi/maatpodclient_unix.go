//go:build darwin || linux

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func nativePodRoutingLedger(st routerstore.Store) (*schedule.Ledger, error) {
	ordinary := schedule.NewLedger(st)
	pod := os.Getenv("SIRSI_MAAT_POD")
	if pod == "" {
		return ordinary, nil
	}
	canonical, ok := st.(schedule.AtomicDelegationScheduler)
	if !ok {
		return nil, fmt.Errorf("maat: compatible pod delegation client required")
	}
	atomic, ok := st.(schedule.AtomicScheduler)
	if !ok {
		return nil, fmt.Errorf("maat: compatible canonical scheduler required")
	}
	cache, err := schedule.FetchFiniteGrantCache(canonical, pod)
	if err != nil {
		return nil, err
	}
	d, err := cache.Snapshot()
	if err != nil {
		return nil, err
	}
	if len(d.AuthorityAddrs) != 1 {
		return nil, fmt.Errorf("maat: exactly one approved native authority endpoint required")
	}
	cert, key, ca := os.Getenv("SIRSI_MAAT_CLIENT_CERT"), os.Getenv("SIRSI_MAAT_CLIENT_KEY"), os.Getenv("SIRSI_MAAT_CLIENT_CA")
	if cert == "" || key == "" || ca == "" {
		return nil, fmt.Errorf("maat: approved native mutual TLS identity selectors required")
	}
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		return nil, fmt.Errorf("maat: approved client identity unavailable")
	}
	b, err := os.ReadFile(ca)
	if err != nil {
		return nil, fmt.Errorf("maat: native authority CA unavailable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(b) {
		return nil, fmt.Errorf("maat: native authority CA invalid")
	}
	transport, err := schedule.NewHTTPPodTransport(d.AuthorityAddrs[0], &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{pair}})
	if err != nil {
		return nil, err
	}
	client := schedule.PodClient{Pod: pod, Canonical: canonical, Ordinary: ordinary, Transport: transport, Cache: cache}
	return schedule.NewLedger(&schedule.PodRoutingStore{StateStore: st, Client: client, Canonical: atomic}), nil
}
func init() { maatLedgerFromStore = nativePodRoutingLedger }
