// metal_bridge.h — C interface for Metal compute bridge.
// Provides GPU-accelerated parallel SHA-256 hashing via Metal compute shaders.
#ifndef METAL_BRIDGE_H
#define METAL_BRIDGE_H

#include <stdint.h>
#include <stddef.h>

// metal_available returns 1 if a Metal GPU device is available.
int metal_available(void);

// metal_gpu_name returns the Metal device name (caller must free).
const char *metal_gpu_name(void);

// metal_gpu_cores returns the estimated GPU core count.
int metal_gpu_cores(void);

// MetalHashRequest describes a batch of data blocks to hash.
typedef struct {
    const uint8_t **blocks;     // array of pointers to data blocks
    const size_t   *block_lens; // array of block lengths
    int             count;      // number of blocks
} MetalHashRequest;

// MetalHashResult holds the output hashes.
typedef struct {
    uint8_t *hashes;  // count * 32 bytes of SHA-256 output
    int      count;   // number of hashes computed
    int      ok;      // 1 = success
    const char *error; // error message if ok==0 (caller must free)
} MetalHashResult;

// metal_hash_batch computes SHA-256 for multiple data blocks in parallel
// using Metal compute shaders. Falls back to CPU dispatch if shader
// compilation fails.
MetalHashResult metal_hash_batch(MetalHashRequest req);

// metal_free_hash_result releases memory from metal_hash_batch.
void metal_free_hash_result(MetalHashResult *r);

#endif // METAL_BRIDGE_H
