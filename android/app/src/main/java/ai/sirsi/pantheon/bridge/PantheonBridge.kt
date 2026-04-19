package ai.sirsi.pantheon.bridge

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import ai.sirsi.pantheon.models.*

// gomobile generates `mobile.Mobile` class with static methods.
// Import path matches the AAR package.
import mobile.Mobile

/**
 * Bridge between Kotlin and the Go mobile package.
 * All Go functions return JSON strings; this layer deserializes them into Kotlin data classes.
 * Mirrors the iOS PantheonBridge.swift pattern.
 */
object PantheonBridge {

    private val json = Json {
        ignoreUnknownKeys = true
        coerceInputValues = true
        isLenient = true
    }

    // --- Response envelope (matches mobile.Response in Go) ---

    @Serializable
    data class BridgeResponse<T>(
        val ok: Boolean,
        val data: T? = null,
        val error: String? = null,
    )

    // --- Errors ---

    sealed class BridgeError(message: String) : Exception(message) {
        class InvalidJson(detail: String) : BridgeError("Invalid JSON response from Pantheon core: $detail")
        class GoError(detail: String) : BridgeError("Pantheon: $detail")
    }

    // --- Anubis ---

    fun anubisCategories(): List<ScanCategory> {
        val raw = Mobile.anubisCategories()
        return decode(raw)
    }

    suspend fun anubisScan(rootPath: String, categories: List<String> = emptyList()): ScanResult =
        withContext(Dispatchers.IO) {
            val optionsJson = if (categories.isNotEmpty()) {
                json.encodeToString(ScanOptions.serializer(), ScanOptions(categories))
            } else ""
            val raw = Mobile.anubisScan(rootPath, optionsJson)
            decode(raw)
        }

    // --- Ka ---

    suspend fun kaHunt(includeSudo: Boolean = false): List<GhostApp> =
        withContext(Dispatchers.IO) {
            val raw = Mobile.kaHunt(includeSudo)
            decode(raw)
        }

    suspend fun kaEnumerateApps(): List<InstalledApp> =
        withContext(Dispatchers.IO) {
            val raw = Mobile.kaEnumerateApps()
            decode(raw)
        }

    // --- Thoth ---

    suspend fun thothInit(projectRoot: String): ProjectInfo =
        withContext(Dispatchers.IO) {
            val raw = Mobile.thothInit(projectRoot)
            decode(raw)
        }

    suspend fun thothSync(root: String): Map<String, String> =
        withContext(Dispatchers.IO) {
            val optionsJson = json.encodeToString(
                kotlinx.serialization.serializer<Map<String, String>>(),
                mapOf("root" to root),
            )
            val raw = Mobile.thothSync(optionsJson)
            decode(raw)
        }

    suspend fun thothCompact(root: String): Map<String, String> =
        withContext(Dispatchers.IO) {
            val optionsJson = json.encodeToString(
                kotlinx.serialization.serializer<Map<String, String>>(),
                mapOf("repo_root" to root),
            )
            val raw = Mobile.thothCompact(optionsJson)
            decode(raw)
        }

    fun thothDetectProject(root: String): ProjectInfo {
        val raw = Mobile.thothDetectProject(root)
        return decode(raw)
    }

    // --- Seba ---

    suspend fun sebaDetectHardware(): HardwareProfile =
        withContext(Dispatchers.IO) {
            val raw = Mobile.sebaDetectHardware()
            decode(raw)
        }

    suspend fun sebaDetectAccelerators(): AcceleratorProfile =
        withContext(Dispatchers.IO) {
            val raw = Mobile.sebaDetectAccelerators()
            decode(raw)
        }

    // --- Seshat ---

    fun seshatListSources(): List<KnowledgeSource> {
        val raw = Mobile.seshatListSources()
        return decode(raw)
    }

    suspend fun seshatIngest(sources: List<String>, sinceDays: Int = 7): List<IngestResult> =
        withContext(Dispatchers.IO) {
            val optionsJson = json.encodeToString(
                IngestOptions.serializer(),
                IngestOptions(sources, sinceDays),
            )
            val raw = Mobile.seshatIngest(optionsJson)
            decode(raw)
        }

    fun seshatListTargets(): List<KnowledgeSource> {
        val raw = Mobile.seshatListTargets()
        return decode(raw)
    }

    fun seshatListKnowledgeItems(): List<String> {
        val raw = Mobile.seshatListKnowledgeItems()
        return decode(raw)
    }

    // --- Version ---

    fun version(): String = Mobile.version()

    // --- JSON Decoding ---

    private inline fun <reified T> decode(jsonString: String): T {
        val response = json.decodeFromString<BridgeResponse<T>>(jsonString)
        if (!response.ok || response.data == null) {
            throw BridgeError.GoError(response.error ?: "unknown error")
        }
        return response.data
    }

    // --- Internal request types ---

    @Serializable
    private data class ScanOptions(val categories: List<String>)

    @Serializable
    private data class IngestOptions(
        val sources: List<String>,
        @SerialName("since_days") val sinceDays: Int,
    )
}
