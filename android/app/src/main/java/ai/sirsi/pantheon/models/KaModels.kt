package ai.sirsi.pantheon.models

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// --- Ka (Ghost Detection) ---

@Serializable
data class GhostApp(
    @SerialName("app_name") val appName: String,
    @SerialName("bundle_id") val bundleId: String,
    val residuals: List<Residual>,
    @SerialName("total_size") val totalSize: Long,
    @SerialName("total_files") val totalFiles: Int,
    @SerialName("in_launch_services") val inLaunchServices: Boolean,
    @SerialName("detection_method") val detectionMethod: String? = null,
) {
    val id: String get() = appName
    val formattedSize: String get() = formatBytes(totalSize)
}

@Serializable
data class Residual(
    val path: String,
    val type: String,
    @SerialName("size_bytes") val sizeBytes: Long,
    @SerialName("file_count") val fileCount: Int,
    @SerialName("requires_sudo") val requiresSudo: Boolean,
) {
    val id: String get() = path
    val formattedSize: String get() = formatBytes(sizeBytes)
}

@Serializable
data class InstalledApp(
    val name: String,
    @SerialName("bundle_id") val bundleId: String,
    val path: String,
    val version: String? = null,
    val source: String? = null,
    val size: Long,
    @SerialName("last_used") val lastUsed: String? = null,
    @SerialName("is_running") val isRunning: Boolean,
    @SerialName("has_ghosts") val hasGhosts: Boolean,
    @SerialName("ghost_count") val ghostCount: Int,
    @SerialName("ghost_size") val ghostSize: Long,
) {
    val id: String get() = bundleId
}
