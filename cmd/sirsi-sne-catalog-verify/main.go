package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func main() {
	var catalogPath, signaturePath, publicKeyPath, packagesRoot string
	flag.StringVar(&catalogPath, "catalog", "", "signed runtime catalog")
	flag.StringVar(&signaturePath, "signature", "", "detached base64 signature")
	flag.StringVar(&publicKeyPath, "public-key", "", "trusted Ed25519 public key")
	flag.StringVar(&packagesRoot, "packages-root", "", "local package materialization root")
	flag.Parse()
	if catalogPath == "" || signaturePath == "" || publicKeyPath == "" || packagesRoot == "" {
		fatalf("catalog, signature, public-key, and packages-root are required")
	}
	catalog, err := sne.LoadSignedRuntimePackageCatalog(catalogPath, signaturePath, publicKeyPath)
	if err != nil {
		fatalf("verify catalog: %v", err)
	}
	catalog, err = catalog.MaterializePackageRoots(packagesRoot)
	if err != nil {
		fatalf("materialize catalog: %v", err)
	}
	for _, entry := range catalog.Entries {
		if entry.PackageID == "" || entry.PackageRoot == "" {
			fatalf("entry model=%q runtime=%q lacks portable materialization", entry.ModelID, entry.RuntimeID)
		}
	}
	fmt.Printf("accepted=true catalog_id=%s entries=%d packages_root=%s\n", catalog.CatalogID, len(catalog.Entries), packagesRoot)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-catalog-verify: "+format+"\n", args...)
	os.Exit(1)
}
