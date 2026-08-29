package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/sne"
)

type admissionReceipt struct {
	Schema    string            `json:"schema"`
	Status    string            `json:"status"`
	CreatedAt string            `json:"created_at"`
	Identity  map[string]string `json:"identity"`
	Execution struct {
		CacheTopology        string `json:"cache_topology"`
		Mode                 string `json:"mode"`
		ServingCacheCapacity int    `json:"serving_cache_capacity"`
	} `json:"execution"`
	Gates    map[string]string      `json:"gates"`
	Evidence map[string]evidenceRef `json:"evidence"`
	Claim    *struct {
		PerformancePromoted  bool `json:"performance_promoted"`
		RuntimeReadinessOnly bool `json:"runtime_readiness_only"`
	} `json:"claim"`
	ClaimBoundary *struct {
		PerformancePromoted  bool `json:"performance_promoted"`
		RuntimeReadinessOnly bool `json:"runtime_readiness_only"`
	} `json:"claim_boundary"`
}

type evidenceRef struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type productParent struct {
	Schema                        string `json:"schema"`
	API4096PackageSHA256          string `json:"api4096_package_sha256"`
	API4096ServiceSHA256          string `json:"api4096_service_sha256"`
	API4096RuntimeSHA256          string `json:"api4096_runtime_sha256"`
	API4096MLXSHA256              string `json:"api4096_mlx_sha256"`
	API4096MetallibSHA256         string `json:"api4096_metallib_sha256"`
	API4096ManifestSHA256         string `json:"api4096_manifest_sha256"`
	API4096AdmissionReceiptSHA256 string `json:"api4096_admission_receipt_sha256"`
	InheritedArtifactsUnchanged   bool   `json:"inherited_execution_artifacts_unchanged"`
}

type lineageParent struct {
	Schema                     string            `json:"schema"`
	ImmutableParent            string            `json:"immutable_parent"`
	SourceArchive              string            `json:"source_archive"`
	SourceArchiveSHA256        string            `json:"source_archive_sha256"`
	ParentSHA256sSHA256        string            `json:"parent_sha256s_sha256"`
	ParentLineageSHA256        string            `json:"parent_lineage_sha256"`
	PreservedPayloadSHA256     map[string]string `json:"preserved_payload_sha256"`
	Change                     string            `json:"change"`
	ExecutionComponentsChanged bool              `json:"execution_components_changed"`
	ClaimScope                 string            `json:"claim_scope"`
}

type packageParent struct {
	product *productParent
	lineage *lineageParent
}

type promotionPointer struct {
	Schema                 string `json:"schema"`
	PackageRelativePath    string `json:"package_relative_path"`
	PackageSHA256          string `json:"package_sha256"`
	AdmissionReceiptSHA256 string `json:"admission_receipt_sha256"`
}

type modelManifest struct {
	Model struct {
		ID string `json:"id"`
	} `json:"model"`
	Architecture struct {
		CacheTopology string `json:"cache_topology"`
	} `json:"architecture"`
	Qualification struct {
		ServingCacheCapacity int `json:"serving_cache_capacity"`
	} `json:"qualification"`
}

var requiredGates = map[string]string{
	"prompt_logit_parity": "exact", "generated_token_parity": "exact",
	"terminal_logit_parity": "exact", "streaming_equivalence": "exact",
	"cancellation": "pass", "api_32_128_512": "pass", "memory_admission": "pass",
	"swap_admission": "pass", "stop_sequence": "pass", "timeout_recovery": "pass",
	"fifo_fairness": "pass", "realistic_4k_retrieval": "pass",
	"varied_32_128_512": "pass", "structured_streaming": "pass",
	"prefix_multiturn_reset": "pass", "bounded_quality": "pass",
	"composition_wiring": "pass",
}

var requiredV3Evidence = []string{"fairness", "functional", "lengths", "observer_binding", "prefix_session", "prompt_logit", "quality", "retrieval", "stop", "structured", "timeout", "varied_lengths", "wiring"}

func main() {
	current := flag.String("current-catalog", "", "currently signed runtime catalog")
	signature := flag.String("current-signature", "", "detached signature for current catalog")
	publicKey := flag.String("public-key", "", "trusted catalog public key")
	packageRoot := flag.String("package", "", "promoted SNE product candidate")
	receiptPath := flag.String("admission-receipt", "", "accepted API4096 v2 receipt")
	pointerPath := flag.String("promotion-pointer", "", "active SNE launch-candidate pointer")
	runtimeID := flag.String("runtime-id", "", "stable explicit Pantheon runtime identity")
	catalogID := flag.String("catalog-id", "", "successor catalog identity")
	output := flag.String("output", "", "exclusive unsigned catalog candidate")
	flag.Parse()
	for _, value := range []string{*current, *signature, *publicKey, *packageRoot, *receiptPath, *pointerPath, *runtimeID, *catalogID, *output} {
		if strings.TrimSpace(value) == "" {
			fail(64, "all arguments are required")
		}
	}
	if _, err := os.Lstat(*output); err == nil {
		fail(65, "output already exists")
	} else if !os.IsNotExist(err) {
		fail(65, "inspect output: %v", err)
	}
	catalog, err := sne.LoadSignedRuntimePackageCatalog(*current, *signature, *publicKey)
	if err != nil {
		fail(66, "verify current signed catalog: %v", err)
	}
	entry, err := buildEntry(*packageRoot, *receiptPath, *pointerPath, *runtimeID, sne.VerifyRuntimePackageBoundary)
	if err != nil {
		fail(67, "%v", err)
	}
	if *catalogID == catalog.CatalogID {
		fail(68, "successor catalog_id must differ from current catalog")
	}
	for _, existing := range catalog.Entries {
		if existing.ModelID == entry.ModelID && existing.EffectiveRuntimeID() == entry.RuntimeID {
			fail(68, "catalog already contains this model/runtime identity")
		}
		if existing.PackageID == entry.PackageID {
			fail(68, "catalog already contains this package identity")
		}
	}
	catalog.CatalogID = *catalogID
	catalog.Entries = append(catalog.Entries, entry)
	sort.Slice(catalog.Entries, func(i, j int) bool {
		left, right := catalog.Entries[i], catalog.Entries[j]
		if left.ModelID != right.ModelID {
			return left.ModelID < right.ModelID
		}
		return left.EffectiveRuntimeID() < right.EffectiveRuntimeID()
	})
	if err := writeExclusiveJSON(*output, catalog); err != nil {
		fail(69, "write catalog candidate: %v", err)
	}
	fmt.Printf("sne_catalog_candidate created=true signed=false installed=false activated=false model=%s runtime=%s output=%s\n", entry.ModelID, entry.RuntimeID, *output)
}

func buildEntry(packageRoot, receiptPath, pointerPath, runtimeID string, verify func(sne.RuntimePackage) error) (sne.RuntimePackage, error) {
	root, err := filepath.Abs(packageRoot)
	if err != nil {
		return sne.RuntimePackage{}, err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return sne.RuntimePackage{}, errors.New("package root is unavailable, non-directory, or symlinked")
	}
	packageID := filepath.Base(root)
	if packageID == "." || packageID == ".." || strings.ContainsAny(packageID, "/\\") {
		return sne.RuntimePackage{}, errors.New("unsafe package identity")
	}

	var receipt admissionReceipt
	if err = loadStrict(receiptPath, &receipt); err != nil {
		return sne.RuntimePackage{}, fmt.Errorf("admission receipt: %w", err)
	}
	if receipt.Status != "accepted" {
		return sne.RuntimePackage{}, errors.New("receipt is not accepted API4096 v2 runtime-readiness evidence")
	}
	switch receipt.Schema {
	case "sne.v2.api4096-parent-admission.v2":
		if receipt.Claim == nil || receipt.Claim.PerformancePromoted || !receipt.Claim.RuntimeReadinessOnly {
			return sne.RuntimePackage{}, errors.New("receipt is not accepted API4096 v2 runtime-readiness evidence")
		}
	case "sne.v2.api4096-parent-admission.v3":
		if receipt.Claim != nil || receipt.ClaimBoundary == nil || receipt.ClaimBoundary.PerformancePromoted || !receipt.ClaimBoundary.RuntimeReadinessOnly {
			return sne.RuntimePackage{}, errors.New("API4096 v3 receipt has an invalid claim boundary")
		}
		if _, err = time.Parse(time.RFC3339Nano, receipt.CreatedAt); err != nil {
			return sne.RuntimePackage{}, errors.New("API4096 v3 receipt has an invalid creation time")
		}
		if receipt.Execution.Mode != "mtp" || receipt.Execution.CacheTopology == "" || receipt.Execution.ServingCacheCapacity < 4096 {
			return sne.RuntimePackage{}, errors.New("API4096 v3 receipt lacks practical execution identity")
		}
		for _, name := range requiredV3Evidence {
			reference, ok := receipt.Evidence[name]
			if !ok || !filepath.IsAbs(reference.Path) || len(reference.SHA256) != 64 {
				return sne.RuntimePackage{}, fmt.Errorf("API4096 v3 receipt evidence %s is incomplete", name)
			}
			digest, digestErr := digestFile(reference.Path)
			if digestErr != nil || digest != reference.SHA256 {
				return sne.RuntimePackage{}, fmt.Errorf("API4096 v3 receipt evidence %s hash mismatch", name)
			}
		}
	default:
		return sne.RuntimePackage{}, errors.New("unsupported API4096 admission receipt schema")
	}
	for gate, expected := range requiredGates {
		if receipt.Gates[gate] != expected {
			return sne.RuntimePackage{}, fmt.Errorf("receipt gate %s is not %s", gate, expected)
		}
	}

	parent, err := loadPackageParent(filepath.Join(root, "manifests/PARENT.json"))
	if err != nil {
		return sne.RuntimePackage{}, fmt.Errorf("product parent: %w", err)
	}
	var pointer promotionPointer
	if err = loadStrict(pointerPath, &pointer); err != nil {
		return sne.RuntimePackage{}, fmt.Errorf("promotion pointer: %w", err)
	}
	if pointer.Schema != "sne.active-launch-candidate.v1" {
		return sne.RuntimePackage{}, errors.New("unsupported promotion pointer")
	}
	receiptSHA, err := digestFile(receiptPath)
	if err != nil {
		return sne.RuntimePackage{}, err
	}
	packageSHA, err := digestFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return sne.RuntimePackage{}, err
	}
	if pointer.PackageSHA256 != packageSHA || pointer.AdmissionReceiptSHA256 != receiptSHA {
		return sne.RuntimePackage{}, errors.New("promotion, package, and admission identities disagree")
	}
	if receipt.Schema == "sne.v2.api4096-parent-admission.v3" && receipt.Identity["package_sha256"] != packageSHA {
		return sne.RuntimePackage{}, errors.New("package seal disagrees with admission receipt")
	}
	sealed := map[string]string{}
	if receipt.Schema == "sne.v2.api4096-parent-admission.v3" {
		sealed, err = verifyPackageSeal(root)
		if err != nil {
			return sne.RuntimePackage{}, fmt.Errorf("package seal: %w", err)
		}
	}
	if parent.product != nil && parent.product.API4096AdmissionReceiptSHA256 != receiptSHA {
		return sne.RuntimePackage{}, errors.New("product ancestry and admission receipt disagree")
	}
	if filepath.Base(filepath.Clean(pointer.PackageRelativePath)) != packageID {
		return sne.RuntimePackage{}, errors.New("promotion pointer names another package")
	}

	paths := map[string]string{
		"service_sha256": "bin/sned", "runtime_sha256": "lib/runtime/libsirsi_native_runtime.dylib",
		"mlx_sha256": "lib/libmlx.dylib", "metallib_sha256": "share/mlx.metallib",
		"manifest_sha256": "manifests/model.json", "jaccl_sha256": "lib/libjaccl.dylib",
	}
	actual := map[string]string{}
	for key, relative := range paths {
		actual[key], err = digestFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return sne.RuntimePackage{}, fmt.Errorf("%s: %w", key, err)
		}
		if receipt.Schema == "sne.v2.api4096-parent-admission.v3" && sealed[relative] != actual[key] {
			return sne.RuntimePackage{}, fmt.Errorf("package seal does not bind %s", relative)
		}
	}
	bindings := []struct{ key, receipt string }{
		{"service_sha256", receipt.Identity["service_sha256"]},
		{"runtime_sha256", receipt.Identity["runtime_sha256"]},
		{"mlx_sha256", receipt.Identity["mlx_sha256"]},
		{"metallib_sha256", receipt.Identity["metallib_sha256"]},
		{"manifest_sha256", receipt.Identity["manifest_sha256"]},
	}
	for _, binding := range bindings {
		if actual[binding.key] != binding.receipt {
			return sne.RuntimePackage{}, fmt.Errorf("%s identity disagrees across package and receipt", binding.key)
		}
	}
	if parent.product != nil {
		productBindings := map[string]string{
			"service_sha256":  parent.product.API4096ServiceSHA256,
			"runtime_sha256":  parent.product.API4096RuntimeSHA256,
			"mlx_sha256":      parent.product.API4096MLXSHA256,
			"metallib_sha256": parent.product.API4096MetallibSHA256,
			"manifest_sha256": parent.product.API4096ManifestSHA256,
		}
		for key, expected := range productBindings {
			if actual[key] != expected {
				return sne.RuntimePackage{}, fmt.Errorf("%s identity disagrees with product ancestry", key)
			}
		}
	}

	manifest, err := loadBoundModelManifest(
		filepath.Join(root, "manifests/model.json"),
		receipt.Schema == "sne.v2.api4096-parent-admission.v3",
	)
	if err != nil {
		return sne.RuntimePackage{}, fmt.Errorf("model manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Model.ID) == "" || manifest.Architecture.CacheTopology == "" || manifest.Qualification.ServingCacheCapacity < 4096 {
		return sne.RuntimePackage{}, errors.New("model manifest lacks practical API4096 identity/capacity")
	}
	entry := sne.RuntimePackage{ModelID: manifest.Model.ID, RuntimeID: strings.TrimSpace(runtimeID), PackageID: packageID,
		RuntimeSHA256: actual["service_sha256"], NativeRuntimeSHA256: actual["runtime_sha256"],
		MLXDylibSHA256: actual["mlx_sha256"], MetallibSHA256: actual["metallib_sha256"], JACCLSHA256: actual["jaccl_sha256"],
		CacheTopology: manifest.Architecture.CacheTopology, ServingCacheCapacity: manifest.Qualification.ServingCacheCapacity}
	if entry.RuntimeID == "" {
		return sne.RuntimePackage{}, errors.New("runtime_id is empty")
	}
	entry.PackageRoot = root
	entry.PackageID = ""
	if err := verify(entry); err != nil {
		return sne.RuntimePackage{}, fmt.Errorf("package boundary: %w", err)
	}
	entry.PackageRoot = ""
	entry.PackageID = packageID
	return entry, nil
}

func loadBoundModelManifest(path string, requireSchema bool) (modelManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return modelManifest{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var document map[string]json.RawMessage
	if err := decoder.Decode(&document); err != nil {
		return modelManifest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return modelManifest{}, errors.New("multiple JSON values")
	}
	var schema string
	schemaDocument, hasSchema := document["schema_version"]
	if hasSchema {
		if err := json.Unmarshal(schemaDocument, &schema); err != nil {
			return modelManifest{}, errors.New("unsupported model manifest schema")
		}
	}
	if requireSchema && (!hasSchema || schema != "sne.model-manifest.v0") {
		return modelManifest{}, errors.New("unsupported model manifest schema")
	}
	if !requireSchema && hasSchema && schema != "sne.model-manifest.v0" {
		return modelManifest{}, errors.New("unsupported model manifest schema")
	}
	var manifest modelManifest
	if err := json.Unmarshal(document["model"], &manifest.Model); err != nil {
		return modelManifest{}, err
	}
	if err := json.Unmarshal(document["architecture"], &manifest.Architecture); err != nil {
		return modelManifest{}, err
	}
	if err := json.Unmarshal(document["qualification"], &manifest.Qualification); err != nil {
		return modelManifest{}, err
	}
	return manifest, nil
}

func verifyPackageSeal(root string) (map[string]string, error) {
	file, err := os.Open(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	sealed := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.SplitN(scanner.Text(), "  ", 2)
		if len(fields) != 2 || len(fields[0]) != 64 || strings.TrimSpace(fields[1]) == "" {
			return nil, errors.New("invalid SHA256SUMS entry")
		}
		relative := strings.TrimPrefix(fields[1], "./")
		clean := filepath.Clean(relative)
		if clean != relative || filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, errors.New("unsafe SHA256SUMS path")
		}
		if _, exists := sealed[clean]; exists {
			return nil, errors.New("duplicate SHA256SUMS path")
		}
		digest, err := digestFile(filepath.Join(root, clean))
		if err != nil || digest != fields[0] {
			return nil, fmt.Errorf("SHA256SUMS mismatch for %s", clean)
		}
		sealed[clean] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return sealed, nil
}

func loadPackageParent(path string) (packageParent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageParent{}, err
	}
	var header struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return packageParent{}, err
	}
	switch header.Schema {
	case "sne.api4096-product-parent.v1":
		var parent productParent
		if err := loadStrict(path, &parent); err != nil {
			return packageParent{}, err
		}
		if !parent.InheritedArtifactsUnchanged {
			return packageParent{}, errors.New("package lacks unchanged API4096 product ancestry")
		}
		return packageParent{product: &parent}, nil
	case "sirsi.sne.package-lineage.v1":
		var parent lineageParent
		if err := loadStrict(path, &parent); err != nil {
			return packageParent{}, err
		}
		if parent.ImmutableParent == "" || parent.SourceArchiveSHA256 == "" || parent.ParentSHA256sSHA256 == "" || parent.ParentLineageSHA256 == "" || parent.ExecutionComponentsChanged || len(parent.PreservedPayloadSHA256) == 0 {
			return packageParent{}, errors.New("package lacks unchanged immutable lineage")
		}
		return packageParent{lineage: &parent}, nil
	default:
		return packageParent{}, errors.New("unsupported package parent schema")
	}
}

func loadStrict(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("multiple JSON values")
	}
	return nil
}

func digestFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func writeExclusiveJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func fail(code int, format string, args ...any) {
	fmt.Fprintf(os.Stderr, "sirsi-sne-catalog-candidate: "+format+"\n", args...)
	os.Exit(code)
}
