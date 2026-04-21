package ai.sirsi.pantheon.models

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// --- Stele (Event Ledger) ---

@Serializable
data class SteleEntry(
    val seq: Long,
    val prev: String,
    val deity: String,
    val type: String,
    val scope: String,
    val data: Map<String, String>? = null,
    val ts: String,
    val hash: String,
) {
    val deityGlyph: String get() = deityGlyphs[deity.lowercase()] ?: deity
    val deityName: String get() = deity.replaceFirstChar { it.uppercase() }

    val typeColorName: String get() = when (type) {
        "governance", "maat_weigh", "maat_pulse", "maat_audit", "maat_heal" -> "governance"
        "commit" -> "commit"
        "tool_use" -> "toolUse"
        "deploy_start", "deploy_end" -> "deploy"
        "drift_check", "neith_drift" -> "driftCheck"
        "failed" -> "error"
        else -> "default"
    }

    companion object {
        val deityGlyphs = mapOf(
            "anubis" to "\uD80C\uDCE3",
            "maat" to "\uD80C\uDD84",
            "thoth" to "\uD80C\uDC5F",
            "ra" to "\uD80C\uDDF6",
            "neith" to "\uD80C\uDC6F",
            "seba" to "\uD80C\uDDFD",
            "seshat" to "\uD80C\uDC46",
            "isis" to "\uD80C\uDC50",
            "osiris" to "\uD80C\uDC79",
            "horus" to "\uD80C\uDC80",
            "ka" to "\uD80C\uDC93",
            "rtk" to "\u26A1",
            "vault" to "\uD83C\uDFDB\uFE0F",
        )
    }
}

@Serializable
data class SteleStats(
    val totalEntries: Int,
    val deityCounts: Map<String, Int>,
    val typeCounts: Map<String, Int>,
    val firstTs: String? = null,
    val lastTs: String? = null,
)

@Serializable
data class SteleVerifyResult(
    val status: String,
    val chainLength: Int,
    val totalCount: Int,
    val breaks: List<String>,
    val verifiedAt: String,
) {
    val isVerified: Boolean get() = status == "verified"
}
