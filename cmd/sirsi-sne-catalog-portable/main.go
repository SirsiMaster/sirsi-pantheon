package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func main() {
	var inputPath, outputPath, packagesRoot string
	flag.StringVar(&inputPath, "input", "", "installed runtime catalog")
	flag.StringVar(&outputPath, "output", "", "portable runtime catalog output")
	flag.StringVar(&packagesRoot, "packages-root", "", "governed package root")
	flag.Parse()
	if inputPath == "" || outputPath == "" || packagesRoot == "" {
		fatalf("input, output, and packages-root are required")
	}
	catalog, err := sne.LoadRuntimePackageCatalog(inputPath)
	if err != nil {
		fatalf("load installed catalog: %v", err)
	}
	portable, err := catalog.PortablePackageCatalog(packagesRoot)
	if err != nil {
		fatalf("make catalog portable: %v", err)
	}
	data, err := json.MarshalIndent(portable, "", "  ")
	if err != nil {
		fatalf("encode portable catalog: %v", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(outputPath, data); err != nil {
		fatalf("write portable catalog: %v", err)
	}
	fmt.Printf("portable catalog=%s entries=%d source=%s\n", outputPath, len(portable.Entries), inputPath)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sne-portable-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-catalog-portable: "+format+"\n", args...)
	os.Exit(1)
}
