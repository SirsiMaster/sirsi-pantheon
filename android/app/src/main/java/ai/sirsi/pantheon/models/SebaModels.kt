package ai.sirsi.pantheon.models

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// --- Seba (Hardware Detection) ---

@Serializable
data class HardwareProfile(
    @SerialName("cpu_cores") val cpuCores: Int,
    @SerialName("cpu_model") val cpuModel: String,
    @SerialName("cpu_arch") val cpuArch: String,
    @SerialName("total_ram") val totalRam: Long,
    val gpu: GPUInfo? = null,
    @SerialName("neural_engine") val neuralEngine: Boolean? = null,
    val os: String? = null,
    val kernel: String? = null,
) {
    val formattedRAM: String get() = formatBytes(totalRam)
}

@Serializable
data class GPUInfo(
    val type: String,
    val name: String,
    val vram: Long? = null,
    @SerialName("metal_family") val metalFamily: String? = null,
    @SerialName("cuda_version") val cudaVersion: String? = null,
    @SerialName("driver_ver") val driverVer: String? = null,
    val compute: String? = null,
)

@Serializable
data class AcceleratorProfile(
    @SerialName("has_gpu") val hasGpu: Boolean,
    @SerialName("gpu_cores") val gpuCores: Int? = null,
    @SerialName("gpu_vendor") val gpuVendor: String? = null,
    @SerialName("has_ane") val hasAne: Boolean,
    @SerialName("ane_cores") val aneCores: Int? = null,
    @SerialName("has_metal") val hasMetal: Boolean,
    @SerialName("has_cuda") val hasCuda: Boolean,
    @SerialName("has_rocm") val hasRocm: Boolean,
    @SerialName("has_oneapi") val hasOneapi: Boolean,
    @SerialName("cpu_cores") val cpuCores: Int,
    @SerialName("mem_bandwidth") val memBandwidth: String? = null,
    @SerialName("unified_memory") val unifiedMemory: Boolean? = null,
    val routing: String? = null,
)
