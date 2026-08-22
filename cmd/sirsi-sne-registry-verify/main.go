package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

func main() {
	admissionPath := flag.String("admission", "", "model admission registry")
	readinessPath := flag.String("readiness", "", "model readiness registry")
	catalogEntry := flag.String("catalog-entry", "", "exact catalog entry")
	policy := flag.String("policy", sne.ReadinessIdentity, "identity, correctness, performance, or release")
	expectedManifest := flag.String("expected-manifest-sha256", "", "optional exact model manifest SHA-256")
	expectedArtifactSet := flag.String("expected-artifact-set-sha256", "", "optional exact artifact-set SHA-256")
	flag.Parse()

	for label, value := range map[string]string{
		"admission": *admissionPath, "readiness": *readinessPath, "catalog-entry": *catalogEntry,
	} {
		if strings.TrimSpace(value) == "" {
			fail("%s is required", label)
		}
	}
	registry, err := sne.LoadModelAdmissionRegistry(*admissionPath)
	if err != nil {
		fail("admission: %v", err)
	}
	var selected *sne.ModelAdmission
	for index := range registry.Entries {
		if registry.Entries[index].CatalogEntry == *catalogEntry {
			selected = &registry.Entries[index]
			break
		}
	}
	if selected == nil {
		fail("catalog entry %q is absent", *catalogEntry)
	}
	if *expectedManifest != "" && selected.ManifestSHA256 != *expectedManifest {
		fail("manifest SHA-256 mismatch")
	}
	if *expectedArtifactSet != "" && selected.ArtifactSetSHA256 != *expectedArtifactSet {
		fail("artifact-set SHA-256 mismatch")
	}
	readiness, err := sne.EvaluateModelReadiness(*readinessPath, registry, *catalogEntry, *policy)
	if err != nil {
		fail("readiness: %v", err)
	}
	fmt.Printf("sne_registry verified=true catalog_id=%s catalog_entry=%s policy=%s qualification=%s exactness=%s clean100=%s lifecycle=%s pantheon=%s disposition=%s manifest_sha256=%s artifact_set_sha256=%s\n",
		registry.CatalogID, selected.CatalogEntry, *policy, selected.Qualification, readiness.Exactness,
		readiness.Clean100, readiness.SupervisedLifecycle, readiness.Pantheon, readiness.Disposition,
		selected.ManifestSHA256, selected.ArtifactSetSHA256)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-registry-verify: "+format+"\n", args...)
	os.Exit(1)
}
