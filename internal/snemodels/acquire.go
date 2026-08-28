package snemodels

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Progress struct {
	CatalogEntry string `json:"catalog_entry"`
	Path         string `json:"path"`
	FilesDone    int    `json:"files_done"`
	FilesTotal   int    `json:"files_total"`
	BytesDone    int64  `json:"bytes_done"`
	BytesTotal   int64  `json:"bytes_total"`
	ResumedBytes int64  `json:"resumed_bytes"`
}

type AcquireOptions struct {
	Destination string
	BearerToken string
	BaseURL     string
	Client      *http.Client
	Progress    func(Progress)
}

type AcquireResult struct {
	CatalogEntry string `json:"catalog_entry"`
	SourceDir    string `json:"source_dir"`
	Repository   string `json:"repository"`
	Revision     string `json:"revision"`
	Files        int    `json:"files"`
	Bytes        int64  `json:"bytes"`
}

func Acquire(ctx context.Context, entry SourceEntry, options AcquireOptions) (AcquireResult, error) {
	if err := entry.validate(); err != nil {
		return AcquireResult{}, err
	}
	if strings.TrimSpace(options.Destination) == "" {
		return AcquireResult{}, fmt.Errorf("SNE acquisition destination is required")
	}
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = entry.BaseURL
		if baseURL == "" {
			baseURL = "https://huggingface.co"
		}
	}
	base, err := url.Parse(baseURL)
	if err != nil || (base.Scheme != "https" && !(base.Scheme == "http" && isLoopbackHost(base.Hostname()))) {
		return AcquireResult{}, fmt.Errorf("SNE acquisition requires HTTPS or a loopback test source")
	}
	client := options.Client
	if client == nil {
		client = secureHTTPClient(base.Hostname(), entry.Provider)
	}
	destination, err := filepath.Abs(options.Destination)
	if err != nil {
		return AcquireResult{}, fmt.Errorf("resolve SNE acquisition destination: %w", err)
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return AcquireResult{}, fmt.Errorf("create SNE acquisition destination: %w", err)
	}
	files := sortedSourceFiles(entry.Files)
	progress := Progress{CatalogEntry: entry.CatalogEntry, FilesTotal: len(files)}
	for _, file := range files {
		progress.BytesTotal += file.SizeBytes
	}
	for _, file := range files {
		artifactURL, err := entry.ResolveURL(baseURL, file.Path)
		if err != nil {
			return AcquireResult{}, err
		}
		resumed, err := acquireFile(ctx, client, artifactURL, filepath.Join(destination, file.Path), file, options.BearerToken)
		if err != nil {
			return AcquireResult{}, fmt.Errorf("acquire %s: %w", file.Path, err)
		}
		progress.Path = file.Path
		progress.FilesDone++
		progress.BytesDone += file.SizeBytes
		progress.ResumedBytes += resumed
		if options.Progress != nil {
			options.Progress(progress)
		}
	}
	return AcquireResult{CatalogEntry: entry.CatalogEntry, SourceDir: destination, Repository: entry.Repository, Revision: entry.Revision, Files: len(files), Bytes: progress.BytesDone}, nil
}

func acquireFile(ctx context.Context, client *http.Client, sourceURL, destination string, expected SourceFile, token string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return 0, err
	}
	if err := verifyDownloadedFile(destination, expected); err == nil {
		return expected.SizeBytes, nil
	}
	partial := destination + ".partial"
	file, err := os.OpenFile(partial, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return 0, err
	}
	offset := info.Size()
	if offset < 0 || offset > expected.SizeBytes {
		if err = file.Truncate(0); err != nil {
			file.Close()
			return 0, err
		}
		offset = 0
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		file.Close()
		return 0, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if offset > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	response, err := client.Do(request)
	if err != nil {
		file.Close()
		return offset, err
	}
	defer response.Body.Close()
	if offset > 0 && response.StatusCode == http.StatusOK {
		if err := file.Truncate(0); err != nil {
			file.Close()
			return offset, err
		}
		offset = 0
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		file.Close()
		return offset, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.StatusCode == http.StatusPartialContent && !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(offset, 10)+"-") {
		file.Close()
		return offset, fmt.Errorf("server returned an incompatible content range")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		return offset, err
	}
	written, copyErr := io.CopyN(file, response.Body, expected.SizeBytes-offset)
	if copyErr == nil && written != expected.SizeBytes-offset {
		copyErr = io.ErrUnexpectedEOF
	}
	if copyErr == nil {
		copyErr = file.Sync()
	}
	closeErr := file.Close()
	if copyErr != nil {
		return offset, copyErr
	}
	if closeErr != nil {
		return offset, closeErr
	}
	if err := verifyDownloadedFile(partial, expected); err != nil {
		_ = os.Remove(partial)
		return offset, err
	}
	if err := os.Rename(partial, destination); err != nil {
		return offset, err
	}
	return offset, nil
}

func verifyDownloadedFile(path string, expected SourceFile) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != expected.SizeBytes {
		return fmt.Errorf("downloaded artifact size mismatch")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("downloaded artifact SHA-256 mismatch")
	}
	return nil
}

func secureHTTPClient(initialHost, provider string) *http.Client {
	return &http.Client{
		Timeout: 24 * time.Hour,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			if request.URL.Scheme != "https" || !allowedRedirectHost(initialHost, request.URL.Hostname(), provider) {
				return fmt.Errorf("unsafe SNE model redirect")
			}
			return nil
		},
	}
}

func allowedRedirectHost(initial, host, provider string) bool {
	host = strings.ToLower(host)
	initial = strings.ToLower(initial)
	if host == initial {
		return true
	}
	if provider != "huggingface" {
		return false
	}
	for _, suffix := range []string{".huggingface.co", ".hf.co", ".xethub.hf.co", ".amazonaws.com", ".cloudfront.net"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
