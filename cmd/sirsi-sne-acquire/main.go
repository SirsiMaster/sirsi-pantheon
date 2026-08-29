package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/snemodels"
)

func main() {
	sourceCatalog := flag.String("source-catalog", "", "signed SNE source catalog")
	catalogEntry := flag.String("catalog-entry", "", "exact SNE catalog entry")
	destination := flag.String("destination", "", "prepared-source destination directory")
	baseURL := flag.String("base-url", "", "test-only provider endpoint override; production origins come from the signed source catalog")
	tokenEnvironment := flag.String("token-env", "HF_TOKEN", "environment variable containing the provider bearer token")
	jsonProgress := flag.Bool("json-progress", false, "emit JSON progress records")
	flag.Parse()

	catalog, err := snemodels.LoadSourceCatalog(*sourceCatalog)
	if err != nil {
		fail(err)
	}
	entry, err := catalog.Resolve(*catalogEntry)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	options := snemodels.AcquireOptions{Destination: *destination, BearerToken: os.Getenv(*tokenEnvironment), BaseURL: *baseURL}
	if *jsonProgress {
		options.Progress = func(progress snemodels.Progress) {
			_ = encoder.Encode(map[string]any{"type": "progress", "progress": progress})
		}
	}
	result, err := snemodels.Acquire(context.Background(), entry, options)
	if err != nil {
		fail(err)
	}
	if err := encoder.Encode(map[string]any{"type": "result", "catalog_id": catalog.CatalogID, "result": result}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
