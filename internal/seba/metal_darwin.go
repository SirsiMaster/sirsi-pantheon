//go:build darwin && cgo

// Package seba — metal_darwin.go provides CGO bridge to Metal compute shaders.
// Enables GPU-accelerated parallel SHA-256 hashing on Apple Silicon.
package seba

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Metal -framework CoreGraphics
#include "metal_bridge_darwin.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"crypto/sha256"
	"fmt"
	"unsafe"
)

// cpuSHA256 computes SHA-256 on the CPU. Used for empty blocks that
// Metal cannot handle (zero-length buffer allocation fails).
func cpuSHA256(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// metalAvailable returns true if a Metal GPU device exists.
func metalAvailable() bool {
	return C.metal_available() == 1
}

// metalGPUName returns the Metal device name.
func metalGPUName() string {
	cName := C.metal_gpu_name()
	defer C.free(unsafe.Pointer(cName))
	return C.GoString(cName)
}

// metalGPUCores returns the max threadgroup size (proxy for GPU parallelism).
func metalGPUCores() int {
	return int(C.metal_gpu_cores())
}

// MetalHashBatch computes SHA-256 for multiple data blocks in parallel on the GPU.
// Each block is hashed independently — ideal for dedup scanning, integrity checks,
// and any workload that needs many independent hashes.
//
// CGO pointer rules require that C functions cannot receive Go pointers containing
// other Go pointers. We allocate the pointer array and copy each block into C memory.
//
// Empty blocks (len=0) are hashed on the CPU — Metal cannot allocate zero-length buffers.
func MetalHashBatch(blocks [][]byte) ([][32]byte, error) {
	n := len(blocks)
	if n == 0 {
		return nil, nil
	}

	// Check if all blocks are empty — avoid Metal dispatch entirely
	allEmpty := true
	for _, b := range blocks {
		if len(b) > 0 {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		hashes := make([][32]byte, n)
		for i, b := range blocks {
			hashes[i] = cpuSHA256(b)
		}
		return hashes, nil
	}

	// Separate empty blocks from non-empty for GPU dispatch.
	// Metal buffer allocation requires length > 0.
	emptyIndices := make(map[int]bool)
	for i, b := range blocks {
		if len(b) == 0 {
			emptyIndices[i] = true
		}
	}

	// If there are empty blocks mixed with non-empty, compute empty hashes
	// on CPU and dispatch the rest to Metal.
	if len(emptyIndices) > 0 {
		// Build non-empty block list for Metal dispatch
		var gpuBlocks [][]byte
		gpuIdx := make([]int, 0, n-len(emptyIndices))
		for i, b := range blocks {
			if !emptyIndices[i] {
				gpuBlocks = append(gpuBlocks, b)
				gpuIdx = append(gpuIdx, i)
			}
		}

		// Dispatch non-empty blocks to Metal
		gpuHashes, err := MetalHashBatch(gpuBlocks)
		if err != nil {
			return nil, err
		}

		// Assemble results: CPU-hashed empty blocks + GPU-hashed non-empty blocks
		hashes := make([][32]byte, n)
		for i := range blocks {
			if emptyIndices[i] {
				hashes[i] = cpuSHA256(blocks[i])
			}
		}
		for j, origIdx := range gpuIdx {
			hashes[origIdx] = gpuHashes[j]
		}
		return hashes, nil
	}

	// Allocate C arrays for block pointers and lengths.
	// This avoids violating CGO's "no Go pointer to Go pointer" rule.
	ptrSize := C.size_t(unsafe.Sizeof((*C.uint8_t)(nil)))
	cBlockPtrs := (*[1 << 30]*C.uint8_t)(C.malloc(C.size_t(n) * ptrSize))[:n:n]
	cLens := (*[1 << 30]C.size_t)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(C.size_t(0)))))[:n:n]

	defer C.free(unsafe.Pointer(&cBlockPtrs[0]))
	defer C.free(unsafe.Pointer(&cLens[0]))

	for i, b := range blocks {
		// All blocks here are non-empty (empty blocks were pre-filtered above).
		cBuf := C.malloc(C.size_t(len(b)))
		C.memcpy(cBuf, unsafe.Pointer(&b[0]), C.size_t(len(b)))
		cBlockPtrs[i] = (*C.uint8_t)(cBuf)
		cLens[i] = C.size_t(len(b))
	}

	// Free individual block copies after dispatch
	defer func() {
		for i := 0; i < n; i++ {
			C.free(unsafe.Pointer(cBlockPtrs[i]))
		}
	}()

	req := C.MetalHashRequest{
		blocks:     (**C.uint8_t)(unsafe.Pointer(&cBlockPtrs[0])),
		block_lens: (*C.size_t)(unsafe.Pointer(&cLens[0])),
		count:      C.int(n),
	}

	result := C.metal_hash_batch(req)
	defer C.metal_free_hash_result(&result)

	if result.ok == 0 {
		errMsg := "metal hash failed"
		if result.error != nil {
			errMsg = C.GoString(result.error)
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	// Convert C output to Go hash array
	hashes := make([][32]byte, result.count)
	if result.hashes != nil {
		src := C.GoBytes(unsafe.Pointer(result.hashes), C.int(result.count)*32)
		for i := 0; i < int(result.count); i++ {
			copy(hashes[i][:], src[i*32:(i+1)*32])
		}
	}

	return hashes, nil
}
