package sne

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RuntimeCatalogFeedSchema = "pantheon.sne-runtime-catalog-feed.v1"

const (
	maxRuntimeCatalogFeedBytes      = 4 << 20
	maxRuntimeCatalogBytes          = 16 << 20
	maxRuntimeCatalogSignatureBytes = 16 << 10
)

type RuntimeCatalogFeed struct {
	SchemaVersion string                    `json:"schema_version"`
	FeedID        string                    `json:"feed_id"`
	Entries       []RuntimeCatalogFeedEntry `json:"entries"`
}

type RuntimeCatalogFeedEntry struct {
	VersionSHA256 string `json:"version_sha256"`
	CatalogURL    string `json:"catalog_url"`
	SignatureURL  string `json:"signature_url"`
}

func VerifyRuntimeCatalogFeedSignature(feedPath, signaturePath, publicKeyPath string) error {
	return verifyDetachedEd25519("SNE runtime catalog feed", feedPath, signaturePath, publicKeyPath)
}

func LoadSignedRuntimeCatalogFeed(feedPath, signaturePath, publicKeyPath string) (RuntimeCatalogFeed, error) {
	if err := VerifyRuntimeCatalogFeedSignature(feedPath, signaturePath, publicKeyPath); err != nil {
		return RuntimeCatalogFeed{}, err
	}
	return loadRuntimeCatalogFeed(feedPath)
}

func FetchAndInstallRuntimeCatalogUpdate(ctx context.Context, client *http.Client, feedURL, feedSignatureURL, publicKeyPath, storeRoot, packagesRoot, version string) (RuntimeCatalogInstallResult, error) {
	feed, err := FetchSignedRuntimeCatalogFeed(ctx, client, feedURL, feedSignatureURL, publicKeyPath)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	entry, err := feed.Resolve(version)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	temporary, err := os.MkdirTemp("", ".sne-catalog-update-")
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	defer os.RemoveAll(temporary)
	catalogPath := filepath.Join(temporary, "runtime-packages.json")
	catalogSignaturePath := catalogPath + ".sig"
	if err := downloadHTTPSFile(ctx, client, entry.CatalogURL, catalogPath, maxRuntimeCatalogBytes); err != nil {
		return RuntimeCatalogInstallResult{}, fmt.Errorf("download SNE runtime catalog: %w", err)
	}
	if err := downloadHTTPSFile(ctx, client, entry.SignatureURL, catalogSignaturePath, maxRuntimeCatalogSignatureBytes); err != nil {
		return RuntimeCatalogInstallResult{}, fmt.Errorf("download SNE runtime catalog signature: %w", err)
	}
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return RuntimeCatalogInstallResult{}, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != entry.VersionSHA256 {
		return RuntimeCatalogInstallResult{}, fmt.Errorf("downloaded SNE runtime catalog digest mismatch")
	}
	return InstallSignedRuntimeCatalog(storeRoot, catalogPath, catalogSignaturePath, publicKeyPath, packagesRoot)
}

func FetchSignedRuntimeCatalogFeed(ctx context.Context, client *http.Client, feedURL, feedSignatureURL, publicKeyPath string) (RuntimeCatalogFeed, error) {
	temporary, err := os.MkdirTemp("", ".sne-catalog-feed-")
	if err != nil {
		return RuntimeCatalogFeed{}, err
	}
	defer os.RemoveAll(temporary)
	feedPath := filepath.Join(temporary, "feed.json")
	feedSignaturePath := feedPath + ".sig"
	if err := downloadHTTPSFile(ctx, client, feedURL, feedPath, maxRuntimeCatalogFeedBytes); err != nil {
		return RuntimeCatalogFeed{}, fmt.Errorf("download SNE catalog feed: %w", err)
	}
	if err := downloadHTTPSFile(ctx, client, feedSignatureURL, feedSignaturePath, maxRuntimeCatalogSignatureBytes); err != nil {
		return RuntimeCatalogFeed{}, fmt.Errorf("download SNE catalog feed signature: %w", err)
	}
	return LoadSignedRuntimeCatalogFeed(feedPath, feedSignaturePath, publicKeyPath)
}

func loadRuntimeCatalogFeed(path string) (RuntimeCatalogFeed, error) {
	file, err := os.Open(path)
	if err != nil {
		return RuntimeCatalogFeed{}, err
	}
	defer file.Close()
	var feed RuntimeCatalogFeed
	decoder := json.NewDecoder(io.LimitReader(file, maxRuntimeCatalogFeedBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&feed); err != nil {
		return RuntimeCatalogFeed{}, fmt.Errorf("decode SNE runtime catalog feed: %w", err)
	}
	if err := ensureRuntimeCatalogEOF(decoder); err != nil {
		return RuntimeCatalogFeed{}, fmt.Errorf("decode SNE runtime catalog feed: %w", err)
	}
	if feed.SchemaVersion != RuntimeCatalogFeedSchema || strings.TrimSpace(feed.FeedID) == "" || len(feed.Entries) == 0 {
		return RuntimeCatalogFeed{}, fmt.Errorf("unsupported SNE runtime catalog feed")
	}
	seen := map[string]struct{}{}
	for index, entry := range feed.Entries {
		if !validSHA256Hex(entry.VersionSHA256) || !validHTTPSURL(entry.CatalogURL) || !validHTTPSURL(entry.SignatureURL) {
			return RuntimeCatalogFeed{}, fmt.Errorf("invalid SNE runtime catalog feed entry %d", index)
		}
		if _, duplicate := seen[entry.VersionSHA256]; duplicate {
			return RuntimeCatalogFeed{}, fmt.Errorf("duplicate SNE runtime catalog feed version")
		}
		seen[entry.VersionSHA256] = struct{}{}
	}
	return feed, nil
}

func (feed RuntimeCatalogFeed) Resolve(version string) (RuntimeCatalogFeedEntry, error) {
	version = strings.TrimSpace(version)
	if !validSHA256Hex(version) {
		return RuntimeCatalogFeedEntry{}, fmt.Errorf("invalid SNE runtime catalog update version")
	}
	for _, entry := range feed.Entries {
		if entry.VersionSHA256 == version {
			return entry, nil
		}
	}
	return RuntimeCatalogFeedEntry{}, fmt.Errorf("SNE runtime catalog update version is not in the signed feed")
}

func validHTTPSURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func downloadHTTPSFile(ctx context.Context, client *http.Client, sourceURL, destination string, maximum int64) error {
	if !validHTTPSURL(sourceURL) {
		return fmt.Errorf("HTTPS URL required")
	}
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL.Scheme != "https" {
		return fmt.Errorf("insecure redirect rejected")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maximum {
		return fmt.Errorf("download exceeds size limit")
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".download-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, copyErr := io.Copy(temporary, io.LimitReader(response.Body, maximum+1))
	if copyErr == nil && written > maximum {
		copyErr = fmt.Errorf("download exceeds size limit")
	}
	if copyErr == nil {
		copyErr = temporary.Sync()
	}
	if closeErr := temporary.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		return copyErr
	}
	return os.Rename(temporaryPath, destination)
}
