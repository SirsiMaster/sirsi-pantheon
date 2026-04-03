//go:build !darwin || !cgo

// Package seba — metal_stub.go provides pure-Go fallback when Metal is unavailable.
// Used on Linux, Windows, and darwin builds without CGO.
package seba

import (
	"crypto/sha256"
	"sync"
)

func metalAvailable() bool { return false }
func metalGPUName() string { return "unavailable" }
func metalGPUCores() int   { return 0 }

// MetalHashBatch computes SHA-256 for multiple blocks using Go goroutines.
// This is the pure-Go fallback when Metal is not available.
func MetalHashBatch(blocks [][]byte) ([][32]byte, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	hashes := make([][32]byte, len(blocks))
	var wg sync.WaitGroup

	for i, b := range blocks {
		wg.Add(1)
		go func(idx int, data []byte) {
			defer wg.Done()
			hashes[idx] = sha256.Sum256(data)
		}(i, b)
	}

	wg.Wait()
	return hashes, nil
}
