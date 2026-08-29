package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("operation required: install, current, rollback, remove, support-install, or support-current")
	}
	switch os.Args[1] {
	case "install":
		install(os.Args[2:])
	case "current":
		current(os.Args[2:])
	case "rollback":
		rollback(os.Args[2:])
	case "remove":
		remove(os.Args[2:])
	case "support-install":
		supportInstall(os.Args[2:])
	case "support-current":
		supportCurrent(os.Args[2:])
	default:
		fatalf("unknown operation %q", os.Args[1])
	}
}

func supportInstall(args []string) {
	flags := flag.NewFlagSet("support-install", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	matrix := flags.String("matrix", "", "signed support matrix")
	signature := flags.String("signature", "", "detached signature")
	publicKey := flags.String("public-key", "", "trusted public key")
	_ = flags.Parse(args)
	require(*storeRoot, *matrix, *signature, *publicKey)
	result, err := sne.InstallSignedSupportMatrix(*storeRoot, *matrix, *signature, *publicKey)
	if err != nil {
		fatalf("support-install: %v", err)
	}
	fmt.Printf("support_installed=true version_sha256=%s previous_sha256=%s\n", result.VersionSHA256, result.PreviousSHA256)
}

func supportCurrent(args []string) {
	flags := flag.NewFlagSet("support-current", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	_ = flags.Parse(args)
	require(*storeRoot)
	version, err := sne.CurrentSupportMatrixVersion(*storeRoot)
	if err != nil {
		fatalf("support-current: %v", err)
	}
	fmt.Printf("support_version_sha256=%s\n", version)
}

func install(args []string) {
	flags := flag.NewFlagSet("install", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	catalog := flags.String("catalog", "", "signed portable catalog")
	signature := flags.String("signature", "", "detached signature")
	publicKey := flags.String("public-key", "", "trusted public key")
	packagesRoot := flags.String("packages-root", "", "SNE packages root")
	_ = flags.Parse(args)
	require(*storeRoot, *catalog, *signature, *publicKey, *packagesRoot)
	result, err := sne.InstallSignedRuntimeCatalog(*storeRoot, *catalog, *signature, *publicKey, *packagesRoot)
	if err != nil {
		fatalf("install: %v", err)
	}
	fmt.Printf("installed=true version_sha256=%s previous_sha256=%s\n", result.VersionSHA256, result.PreviousSHA256)
}

func current(args []string) {
	flags := flag.NewFlagSet("current", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	_ = flags.Parse(args)
	require(*storeRoot)
	version, err := sne.CurrentRuntimeCatalogVersion(*storeRoot)
	if err != nil {
		fatalf("current: %v", err)
	}
	fmt.Printf("version_sha256=%s\n", version)
}

func rollback(args []string) {
	flags := flag.NewFlagSet("rollback", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	version := flags.String("version", "", "catalog SHA-256 version")
	publicKey := flags.String("public-key", "", "trusted public key")
	packagesRoot := flags.String("packages-root", "", "SNE packages root")
	_ = flags.Parse(args)
	require(*storeRoot, *version, *publicKey, *packagesRoot)
	if err := sne.RollbackSignedRuntimeCatalog(*storeRoot, *version, *publicKey, *packagesRoot); err != nil {
		fatalf("rollback: %v", err)
	}
	fmt.Printf("rolled_back=true version_sha256=%s\n", *version)
}

func remove(args []string) {
	flags := flag.NewFlagSet("remove", flag.ExitOnError)
	storeRoot := flags.String("store-root", "", "Pantheon catalog store root")
	version := flags.String("version", "", "inactive catalog SHA-256 version")
	_ = flags.Parse(args)
	require(*storeRoot, *version)
	if err := sne.RemoveInactiveRuntimeCatalog(*storeRoot, *version); err != nil {
		fatalf("remove: %v", err)
	}
	fmt.Printf("removed=true version_sha256=%s\n", *version)
}

func require(values ...string) {
	for _, value := range values {
		if value == "" {
			fatalf("all operation arguments are required")
		}
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-catalog-store: "+format+"\n", args...)
	os.Exit(1)
}
