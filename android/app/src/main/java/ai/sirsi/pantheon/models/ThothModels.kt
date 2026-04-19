package ai.sirsi.pantheon.models

import kotlinx.serialization.Serializable

// --- Thoth (Project Memory) ---

@Serializable
data class ProjectInfo(
    val name: String? = null,
    val language: String? = null,
    val version: String? = null,
    val root: String? = null,
)

@Serializable
data class JournalEntry(
    val number: Int,
    val date: String,
    val title: String,
    val commits: List<CommitInfo>? = null,
) {
    val id: Int get() = number
}

@Serializable
data class CommitInfo(
    val hash: String,
    val subject: String,
    val date: String,
    val files: Int? = null,
    val adds: Int? = null,
    val dels: Int? = null,
) {
    val id: String get() = hash
}
