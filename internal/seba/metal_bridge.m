// metal_bridge.m — Objective-C bridge for Metal GPU compute.
// Implements parallel SHA-256 hashing via Metal compute shaders.
//
// Architecture:
//   1. Input data blocks are packed into a contiguous Metal buffer
//   2. An index buffer maps each block's offset and length
//   3. A Metal compute kernel runs SHA-256 on each block independently
//   4. Output hashes (32 bytes each) are read back from the GPU
//
// On Apple Silicon, Metal has unified memory — no CPU↔GPU copy overhead.

#import <Foundation/Foundation.h>
#import <Metal/Metal.h>
#include "metal_bridge.h"
#include <stdlib.h>
#include <string.h>
#include <CommonCrypto/CommonDigest.h>
#include <dispatch/dispatch.h>

// Singleton Metal device — created once, reused.
static id<MTLDevice> _mtl_device = nil;
static id<MTLCommandQueue> _mtl_queue = nil;
static id<MTLComputePipelineState> _mtl_sha256_pipeline = nil;
static dispatch_once_t _mtl_init_token;
static int _mtl_shader_ok = 0;

// SHA-256 Metal compute shader source.
// Each thread hashes one data block independently.
static NSString *const kSHA256ShaderSource = @""
"#include <metal_stdlib>\n"
"using namespace metal;\n"
"\n"
"// SHA-256 constants\n"
"constant uint K[64] = {\n"
"    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5,\n"
"    0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,\n"
"    0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3,\n"
"    0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,\n"
"    0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc,\n"
"    0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,\n"
"    0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,\n"
"    0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,\n"
"    0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13,\n"
"    0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,\n"
"    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3,\n"
"    0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,\n"
"    0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5,\n"
"    0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,\n"
"    0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,\n"
"    0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2\n"
"};\n"
"\n"
"struct BlockIndex {\n"
"    uint offset;\n"
"    uint length;\n"
"};\n"
"\n"
"static inline uint rotr(uint x, uint n) { return (x >> n) | (x << (32 - n)); }\n"
"static inline uint ch(uint x, uint y, uint z) { return (x & y) ^ (~x & z); }\n"
"static inline uint maj(uint x, uint y, uint z) { return (x & y) ^ (x & z) ^ (y & z); }\n"
"static inline uint sigma0(uint x) { return rotr(x, 2) ^ rotr(x, 13) ^ rotr(x, 22); }\n"
"static inline uint sigma1(uint x) { return rotr(x, 6) ^ rotr(x, 11) ^ rotr(x, 25); }\n"
"static inline uint gamma0(uint x) { return rotr(x, 7) ^ rotr(x, 18) ^ (x >> 3); }\n"
"static inline uint gamma1(uint x) { return rotr(x, 17) ^ rotr(x, 19) ^ (x >> 10); }\n"
"\n"
"kernel void sha256_kernel(\n"
"    device const uint8_t *data       [[buffer(0)]],\n"
"    device const BlockIndex *index   [[buffer(1)]],\n"
"    device uint8_t *output           [[buffer(2)]],\n"
"    uint tid                         [[thread_position_in_grid]]\n"
") {\n"
"    uint off = index[tid].offset;\n"
"    uint len = index[tid].length;\n"
"    \n"
"    // SHA-256 initial hash values\n"
"    uint h0 = 0x6a09e667, h1 = 0xbb67ae85, h2 = 0x3c6ef372, h3 = 0xa54ff53a;\n"
"    uint h4 = 0x510e527f, h5 = 0x9b05688c, h6 = 0x1f83d9ab, h7 = 0x5be0cd19;\n"
"    \n"
"    // Pre-processing: calculate padded length\n"
"    uint bit_len = len * 8;\n"
"    uint padded = ((len + 9 + 63) / 64) * 64;\n"
"    \n"
"    // Process each 64-byte block\n"
"    for (uint block = 0; block < padded; block += 64) {\n"
"        uint w[64];\n"
"        \n"
"        // Load 16 words from data with padding\n"
"        for (uint i = 0; i < 16; i++) {\n"
"            uint byte_pos = block + i * 4;\n"
"            uint word = 0;\n"
"            for (uint b = 0; b < 4; b++) {\n"
"                uint pos = byte_pos + b;\n"
"                uint8_t val = 0;\n"
"                if (pos < len) {\n"
"                    val = data[off + pos];\n"
"                } else if (pos == len) {\n"
"                    val = 0x80;\n"
"                } else if (pos >= padded - 4 && b == 0) {\n"
"                    // Big-endian bit length in last 4 bytes\n"
"                } else if (pos == padded - 4) {\n"
"                    val = (bit_len >> 24) & 0xFF;\n"
"                } else if (pos == padded - 3) {\n"
"                    val = (bit_len >> 16) & 0xFF;\n"
"                } else if (pos == padded - 2) {\n"
"                    val = (bit_len >> 8) & 0xFF;\n"
"                } else if (pos == padded - 1) {\n"
"                    val = bit_len & 0xFF;\n"
"                }\n"
"                word = (word << 8) | val;\n"
"            }\n"
"            w[i] = word;\n"
"        }\n"
"        \n"
"        // Extend to 64 words\n"
"        for (uint i = 16; i < 64; i++) {\n"
"            w[i] = gamma1(w[i-2]) + w[i-7] + gamma0(w[i-15]) + w[i-16];\n"
"        }\n"
"        \n"
"        // Compression\n"
"        uint a = h0, b = h1, c = h2, d = h3;\n"
"        uint e = h4, f = h5, g = h6, h = h7;\n"
"        \n"
"        for (uint i = 0; i < 64; i++) {\n"
"            uint t1 = h + sigma1(e) + ch(e, f, g) + K[i] + w[i];\n"
"            uint t2 = sigma0(a) + maj(a, b, c);\n"
"            h = g; g = f; f = e; e = d + t1;\n"
"            d = c; c = b; b = a; a = t1 + t2;\n"
"        }\n"
"        \n"
"        h0 += a; h1 += b; h2 += c; h3 += d;\n"
"        h4 += e; h5 += f; h6 += g; h7 += h;\n"
"    }\n"
"    \n"
"    // Write 32-byte hash output (big-endian)\n"
"    device uint8_t *out = output + tid * 32;\n"
"    uint hvals[8] = {h0, h1, h2, h3, h4, h5, h6, h7};\n"
"    for (uint i = 0; i < 8; i++) {\n"
"        out[i*4+0] = (hvals[i] >> 24) & 0xFF;\n"
"        out[i*4+1] = (hvals[i] >> 16) & 0xFF;\n"
"        out[i*4+2] = (hvals[i] >> 8) & 0xFF;\n"
"        out[i*4+3] = hvals[i] & 0xFF;\n"
"    }\n"
"}\n";

// Initialize Metal device and compile the SHA-256 shader.
static void metal_init(void) {
    dispatch_once(&_mtl_init_token, ^{
        _mtl_device = MTLCreateSystemDefaultDevice();
        if (_mtl_device == nil) return;

        _mtl_queue = [_mtl_device newCommandQueue];
        if (_mtl_queue == nil) return;

        // Compile SHA-256 shader at runtime
        NSError *error = nil;
        id<MTLLibrary> library = [_mtl_device newLibraryWithSource:kSHA256ShaderSource
                                                           options:nil
                                                             error:&error];
        if (library == nil) {
            NSLog(@"Metal SHA-256 shader compilation failed: %@", error);
            return;
        }

        id<MTLFunction> kernel = [library newFunctionWithName:@"sha256_kernel"];
        if (kernel == nil) return;

        _mtl_sha256_pipeline = [_mtl_device newComputePipelineStateWithFunction:kernel error:&error];
        if (_mtl_sha256_pipeline == nil) {
            NSLog(@"Metal pipeline creation failed: %@", error);
            return;
        }

        _mtl_shader_ok = 1;
    });
}

int metal_available(void) {
    metal_init();
    return (_mtl_device != nil) ? 1 : 0;
}

const char *metal_gpu_name(void) {
    metal_init();
    if (_mtl_device == nil) return strdup("unavailable");
    return strdup([[_mtl_device name] UTF8String]);
}

int metal_gpu_cores(void) {
    metal_init();
    if (_mtl_device == nil) return 0;
    // Apple doesn't expose exact core count via Metal API.
    // Use maxThreadsPerThreadgroup as a proxy for parallelism.
    NSUInteger maxThreads = [_mtl_sha256_pipeline maxTotalThreadsPerThreadgroup];
    return (int)maxThreads;
}

// metal_hash_batch_gpu runs SHA-256 on the GPU via Metal compute shader.
static MetalHashResult metal_hash_batch_gpu(MetalHashRequest req) {
    MetalHashResult result = {0};

    @autoreleasepool {
        // Calculate total data size and build index
        size_t total_size = 0;
        for (int i = 0; i < req.count; i++) {
            total_size += req.block_lens[i];
        }

        // Pack data into a contiguous buffer
        uint8_t *packed = (uint8_t *)malloc(total_size);
        if (!packed) {
            result.ok = 0;
            result.error = strdup("malloc failed for packed data");
            return result;
        }

        // Build block index (offset, length pairs)
        typedef struct { uint32_t offset; uint32_t length; } BlockIdx;
        BlockIdx *index = (BlockIdx *)malloc(req.count * sizeof(BlockIdx));
        if (!index) {
            free(packed);
            result.ok = 0;
            result.error = strdup("malloc failed for index");
            return result;
        }

        size_t cursor = 0;
        for (int i = 0; i < req.count; i++) {
            memcpy(packed + cursor, req.blocks[i], req.block_lens[i]);
            index[i].offset = (uint32_t)cursor;
            index[i].length = (uint32_t)req.block_lens[i];
            cursor += req.block_lens[i];
        }

        // Create Metal buffers (unified memory on Apple Silicon — zero-copy)
        id<MTLBuffer> dataBuf = [_mtl_device newBufferWithBytesNoCopy:packed
                                   length:total_size
                                   options:MTLResourceStorageModeShared
                                   deallocator:^(void *ptr, NSUInteger len) { free(ptr); }];
        if (!dataBuf) {
            // Fallback: allocate with copy
            dataBuf = [_mtl_device newBufferWithBytes:packed
                                              length:total_size
                                             options:MTLResourceStorageModeShared];
            free(packed);
        }

        id<MTLBuffer> indexBuf = [_mtl_device newBufferWithBytes:index
                                   length:req.count * sizeof(BlockIdx)
                                   options:MTLResourceStorageModeShared];
        free(index);

        size_t output_size = (size_t)req.count * 32;
        id<MTLBuffer> outputBuf = [_mtl_device newBufferWithLength:output_size
                                    options:MTLResourceStorageModeShared];

        if (!dataBuf || !indexBuf || !outputBuf) {
            result.ok = 0;
            result.error = strdup("Metal buffer allocation failed");
            return result;
        }

        // Encode and dispatch
        id<MTLCommandBuffer> cmdBuf = [_mtl_queue commandBuffer];
        id<MTLComputeCommandEncoder> encoder = [cmdBuf computeCommandEncoder];

        [encoder setComputePipelineState:_mtl_sha256_pipeline];
        [encoder setBuffer:dataBuf offset:0 atIndex:0];
        [encoder setBuffer:indexBuf offset:0 atIndex:1];
        [encoder setBuffer:outputBuf offset:0 atIndex:2];

        NSUInteger threadCount = (NSUInteger)req.count;
        NSUInteger threadgroupSize = MIN(threadCount, [_mtl_sha256_pipeline maxTotalThreadsPerThreadgroup]);
        MTLSize gridSize = MTLSizeMake(threadCount, 1, 1);
        MTLSize groupSize = MTLSizeMake(threadgroupSize, 1, 1);

        [encoder dispatchThreads:gridSize threadsPerThreadgroup:groupSize];
        [encoder endEncoding];
        [cmdBuf commit];
        [cmdBuf waitUntilCompleted];

        if (cmdBuf.error != nil) {
            result.ok = 0;
            NSString *errMsg = [NSString stringWithFormat:@"Metal dispatch error: %@",
                               cmdBuf.error.localizedDescription];
            result.error = strdup([errMsg UTF8String]);
            return result;
        }

        // Copy results
        result.hashes = (uint8_t *)malloc(output_size);
        memcpy(result.hashes, [outputBuf contents], output_size);
        result.count = req.count;
        result.ok = 1;
    }

    return result;
}

// metal_hash_batch_cpu is the GCD-parallel CPU fallback.
static MetalHashResult metal_hash_batch_cpu(MetalHashRequest req) {
    MetalHashResult result = {0};
    size_t output_size = (size_t)req.count * 32;
    result.hashes = (uint8_t *)malloc(output_size);
    if (!result.hashes) {
        result.ok = 0;
        result.error = strdup("malloc failed");
        return result;
    }

    dispatch_apply(req.count, dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^(size_t i) {
        CC_SHA256(req.blocks[i], (CC_LONG)req.block_lens[i], result.hashes + i * 32);
    });

    result.count = req.count;
    result.ok = 1;
    return result;
}

MetalHashResult metal_hash_batch(MetalHashRequest req) {
    if (req.count <= 0) {
        MetalHashResult r = {0};
        r.ok = 1;
        return r;
    }

    metal_init();

    // Use GPU shader if available, otherwise CPU parallel fallback
    if (_mtl_shader_ok) {
        return metal_hash_batch_gpu(req);
    }
    return metal_hash_batch_cpu(req);
}

void metal_free_hash_result(MetalHashResult *r) {
    if (r == NULL) return;
    if (r->hashes != NULL) {
        free(r->hashes);
        r->hashes = NULL;
    }
    if (r->error != NULL) {
        free((void *)r->error);
        r->error = NULL;
    }
}
