// router-history-readback is a read-only, date-bounded canonical history export.
// It avoids ListAll's full historical body response for migration investigation.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: router-history-readback RFC3339-SINCE")
		os.Exit(2)
	}
	if _, err := time.Parse(time.RFC3339, os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	url := os.Getenv("SIRSI_ROUTER_URL")
	if routerstore.SpoolDir(url) == "" {
		fmt.Fprintln(os.Stderr, "requires the canonical host spool, never a local database or token")
		os.Exit(2)
	}
	rs := routerstore.NewRemoteStore(url, "")
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	rows, err := rs.ListSince(ctx, os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
