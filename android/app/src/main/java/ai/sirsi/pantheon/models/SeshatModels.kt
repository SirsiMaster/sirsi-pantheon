package ai.sirsi.pantheon.models

import kotlinx.serialization.Serializable

// --- Seshat (Knowledge Bridge) ---

@Serializable
data class KnowledgeSource(
    val name: String,
    val description: String,
) {
    val id: String get() = name
}

@Serializable
data class IngestResult(
    val source: String,
    val count: Int,
    val error: String? = null,
) {
    val id: String get() = source
}

@Serializable
data class KnowledgeItem(
    val title: String,
    val summary: String? = null,
    val references: List<KIReference>? = null,
) {
    val id: String get() = title
}

@Serializable
data class KIReference(
    val type: String,
    val value: String,
)

@Serializable
data class Conversation(
    val id: String,
    val title: String? = null,
    @kotlinx.serialization.SerialName("started_at") val startedAt: String? = null,
    @kotlinx.serialization.SerialName("message_count") val messageCount: Int? = null,
    val messages: List<ConversationMessage>? = null,
)

@Serializable
data class ConversationMessage(
    val role: String,
    val content: String,
    val timestamp: String? = null,
) {
    val id: String get() = "$role:${timestamp ?: "unknown"}"
}
