package ai.sirsi.pantheon.models

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// --- Anubis (Jackal Scanner) ---

@Serializable
data class ScanCategory(
    val id: String,
    @SerialName("display_name") val displayName: String,
)

@Serializable
data class ScanResult(
    val findings: List<Finding>,
    @SerialName("total_size") val totalSize: Long,
    @SerialName("rules_ran") val rulesRan: Int,
    @SerialName("rules_with_findings") val rulesWithFindings: Int,
    val errors: List<RuleError>? = null,
    @SerialName("by_category") val byCategory: Map<String, CategorySummary>? = null,
) {
    val formattedSize: String get() = formatBytes(totalSize)
}

@Serializable
data class Finding(
    @SerialName("rule_name") val ruleName: String,
    val category: String,
    val description: String,
    val path: String,
    @SerialName("size_bytes") val sizeBytes: Long,
    @SerialName("file_count") val fileCount: Int,
    val severity: String,
    @SerialName("last_modified") val lastModified: String? = null,
    @SerialName("requires_sudo") val requiresSudo: Boolean,
    @SerialName("is_dir") val isDir: Boolean,
) {
    val id: String get() = "$ruleName:$path"
    val formattedSize: String get() = formatBytes(sizeBytes)
}

@Serializable
data class RuleError(
    @SerialName("rule_name") val ruleName: String,
    val error: String,
)

@Serializable
data class CategorySummary(
    @SerialName("total_size") val totalSize: Long,
    @SerialName("finding_count") val findingCount: Int,
)

/** Formats byte counts into human-readable strings. */
fun formatBytes(bytes: Long): String {
    if (bytes < 1024) return "$bytes B"
    val units = arrayOf("KB", "MB", "GB", "TB")
    var value = bytes.toDouble()
    var unitIndex = -1
    do {
        value /= 1024.0
        unitIndex++
    } while (value >= 1024 && unitIndex < units.size - 1)
    return "%.1f %s".format(value, units[unitIndex])
}
