package ai.sirsi.pantheon.ui.screens

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircularShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.launch
import ai.sirsi.pantheon.bridge.PantheonBridge
import ai.sirsi.pantheon.models.SteleEntry
import ai.sirsi.pantheon.models.SteleStats
import ai.sirsi.pantheon.models.SteleVerifyResult
import ai.sirsi.pantheon.ui.components.DeityHeader
import ai.sirsi.pantheon.ui.components.StatPill
import ai.sirsi.pantheon.ui.theme.PantheonError
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface
import ai.sirsi.pantheon.ui.theme.PantheonSuccess
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary

// Event type colors
private val GovernanceColor = Color(0xFFC8A951)
private val CommitColor = Color(0xFF4CAF50)
private val ToolUseColor = Color(0xFF42A5F5)
private val DeployColor = Color(0xFFAB47BC)
private val DriftCheckColor = Color(0xFFFFA726)
private val ErrorColor = Color(0xFFEF5350)

private fun typeColor(name: String): Color = when (name) {
    "governance" -> GovernanceColor
    "commit" -> CommitColor
    "toolUse" -> ToolUseColor
    "deploy" -> DeployColor
    "driftCheck" -> DriftCheckColor
    "error" -> ErrorColor
    else -> PantheonTextSecondary
}

private data class DeityInfo(val key: String, val glyph: String)

private val allDeities = listOf(
    DeityInfo("anubis", "\uD80C\uDCE3"),
    DeityInfo("maat", "\uD80C\uDD84"),
    DeityInfo("thoth", "\uD80C\uDC5F"),
    DeityInfo("ra", "\uD80C\uDDF6"),
    DeityInfo("neith", "\uD80C\uDC6F"),
    DeityInfo("seba", "\uD80C\uDDFD"),
    DeityInfo("seshat", "\uD80C\uDC46"),
    DeityInfo("isis", "\uD80C\uDC50"),
    DeityInfo("osiris", "\uD80C\uDC79"),
    DeityInfo("horus", "\uD80C\uDC80"),
    DeityInfo("rtk", "\u26A1"),
    DeityInfo("vault", "\uD83C\uDFDB\uFE0F"),
)

/**
 * Stele event ledger dashboard. Shows real-time event bus activity
 * across all deities with hash chain verification.
 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun SteleScreen() {
    var entries by remember { mutableStateOf<List<SteleEntry>>(emptyList()) }
    var stats by remember { mutableStateOf<SteleStats?>(null) }
    var verifyResult by remember { mutableStateOf<SteleVerifyResult?>(null) }
    var isLoading by remember { mutableStateOf(false) }
    var isVerifying by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    var selectedDeity by remember { mutableStateOf<String?>(null) }
    var expandedSeq by remember { mutableStateOf<Long?>(null) }
    val scope = rememberCoroutineScope()

    // Load data on mount
    LaunchedEffect(Unit) {
        isLoading = true
        try {
            stats = PantheonBridge.steleStats()
        } catch (_: Exception) { }
        try {
            entries = PantheonBridge.steleReadRecent(100)
        } catch (e: Exception) {
            errorMessage = e.message ?: "Failed to load events"
        }
        isLoading = false
        // Verify in background
        isVerifying = true
        try {
            verifyResult = PantheonBridge.steleVerify()
        } catch (_: Exception) { }
        isVerifying = false
    }

    val filteredEntries = if (selectedDeity != null) {
        entries.filter { it.deity.lowercase() == selectedDeity }
    } else entries

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 16.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        // Header
        item {
            DeityHeader(
                glyph = "\uD80C\uDCBD",
                name = "Stele",
                subtitle = "Event Ledger",
                description = "Append-only hash-chained event bus for all Pantheon inter-deity communication.",
            )
        }

        // A. Stats Card
        if (stats != null) {
            item {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                    shape = RoundedCornerShape(12.dp),
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            text = "Ledger Overview",
                            style = MaterialTheme.typography.titleMedium,
                            color = PantheonGold,
                        )
                        Spacer(modifier = Modifier.height(12.dp))
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceEvenly,
                        ) {
                            StatPill(label = "Events", value = "${stats!!.totalEntries}")
                            StatPill(
                                label = "Chain",
                                value = when {
                                    isVerifying -> "..."
                                    verifyResult?.isVerified == true -> "Verified"
                                    verifyResult != null -> "Broken"
                                    else -> "?"
                                },
                            )
                            StatPill(
                                label = "Deities",
                                value = "${stats!!.deityCounts.size}",
                            )
                        }
                    }
                }
            }
        }

        // B. Deity Activity Grid
        if (stats != null) {
            item {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                    shape = RoundedCornerShape(12.dp),
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            text = "Deity Activity",
                            style = MaterialTheme.typography.titleMedium,
                            color = PantheonGold,
                        )
                        Spacer(modifier = Modifier.height(12.dp))
                        FlowRow(
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                            verticalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            allDeities.forEach { deity ->
                                val count = stats!!.deityCounts[deity.key] ?: 0
                                val isSelected = selectedDeity == deity.key
                                val alpha = if (count > 0) 1f else 0.4f
                                Column(
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                    modifier = Modifier
                                        .width(72.dp)
                                        .clip(RoundedCornerShape(8.dp))
                                        .then(
                                            if (isSelected)
                                                Modifier.border(1.dp, PantheonGold, RoundedCornerShape(8.dp))
                                            else Modifier
                                        )
                                        .background(
                                            if (isSelected) PantheonGold.copy(alpha = 0.2f)
                                            else Color(0xFF252525)
                                        )
                                        .clickable {
                                            selectedDeity = if (isSelected) null else deity.key
                                        }
                                        .padding(vertical = 8.dp),
                                ) {
                                    Text(
                                        text = deity.glyph,
                                        fontSize = 24.sp,
                                        modifier = Modifier.align(Alignment.CenterHorizontally),
                                        color = Color.White.copy(alpha = alpha),
                                    )
                                    Text(
                                        text = "$count",
                                        style = MaterialTheme.typography.labelSmall,
                                        fontWeight = FontWeight.Bold,
                                        color = if (count > 0) PantheonGold else PantheonTextSecondary,
                                    )
                                }
                            }
                        }
                    }
                }
            }
        }

        // Filter indicator
        if (selectedDeity != null) {
            item {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        text = "Filtered: $selectedDeity",
                        style = MaterialTheme.typography.bodySmall,
                        color = PantheonGold,
                    )
                    Text(
                        text = "Clear",
                        style = MaterialTheme.typography.labelMedium,
                        fontWeight = FontWeight.Bold,
                        color = PantheonGold,
                        modifier = Modifier.clickable { selectedDeity = null },
                    )
                }
            }
        }

        // Error
        if (errorMessage != null) {
            item {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = PantheonError.copy(alpha = 0.1f)),
                    shape = RoundedCornerShape(8.dp),
                ) {
                    Text(
                        text = errorMessage!!,
                        modifier = Modifier.padding(16.dp),
                        color = PantheonError,
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
            }
        }

        // Loading
        if (isLoading) {
            item {
                Box(
                    modifier = Modifier.fillMaxWidth().padding(32.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    CircularProgressIndicator(color = PantheonGold)
                }
            }
        }

        // C. Event Timeline
        if (!isLoading) {
            items(filteredEntries, key = { it.seq }) { entry ->
                val isExpanded = expandedSeq == entry.seq
                SteleEntryRow(
                    entry = entry,
                    isExpanded = isExpanded,
                    onClick = {
                        expandedSeq = if (isExpanded) null else entry.seq
                    },
                )
            }

            if (filteredEntries.isEmpty() && entries.isNotEmpty()) {
                item {
                    Text(
                        text = "No events for this deity.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = PantheonTextSecondary,
                        modifier = Modifier.padding(vertical = 8.dp),
                    )
                }
            }
        }

        // D. Hash Chain Status
        if (verifyResult != null) {
            item {
                HashChainStatusCard(verifyResult!!)
            }
        }
    }
}

@Composable
private fun SteleEntryRow(
    entry: SteleEntry,
    isExpanded: Boolean,
    onClick: () -> Unit,
) {
    val color = typeColor(entry.typeColorName)

    Card(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        colors = CardDefaults.cardColors(containerColor = PantheonSurface),
        shape = RoundedCornerShape(8.dp),
    ) {
        Column(modifier = Modifier.padding(12.dp)) {
            // Main row: deity glyph + name, type badge, timestamp
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(text = entry.deityGlyph, fontSize = 22.sp)
                Spacer(modifier = Modifier.width(8.dp))
                Text(
                    text = entry.deityName,
                    style = MaterialTheme.typography.bodyLarge,
                    fontWeight = FontWeight.Bold,
                )
                Spacer(modifier = Modifier.width(8.dp))
                // Type badge
                Text(
                    text = entry.type,
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Bold,
                    color = color,
                    modifier = Modifier
                        .clip(RoundedCornerShape(50))
                        .background(color.copy(alpha = 0.2f))
                        .padding(horizontal = 8.dp, vertical = 3.dp),
                )
                Spacer(modifier = Modifier.weight(1f))
                Text(
                    text = entry.ts.takeLast(9).take(5),
                    style = MaterialTheme.typography.labelSmall,
                    color = PantheonTextSecondary,
                )
            }

            // Scope
            if (entry.scope.isNotEmpty()) {
                Spacer(modifier = Modifier.height(4.dp))
                Text(
                    text = entry.scope,
                    style = MaterialTheme.typography.bodySmall,
                    color = PantheonTextSecondary,
                )
            }

            // Expanded data payload
            AnimatedVisibility(
                visible = isExpanded,
                enter = expandVertically(),
                exit = shrinkVertically(),
            ) {
                Column(modifier = Modifier.padding(top = 8.dp)) {
                    entry.data?.toSortedMap()?.forEach { (key, value) ->
                        Row(modifier = Modifier.padding(vertical = 1.dp)) {
                            Text(
                                text = key,
                                style = MaterialTheme.typography.labelSmall,
                                fontWeight = FontWeight.Bold,
                                color = PantheonGold,
                            )
                            Spacer(modifier = Modifier.width(6.dp))
                            Text(
                                text = value,
                                style = MaterialTheme.typography.labelSmall,
                                color = PantheonTextSecondary,
                                maxLines = 3,
                                overflow = TextOverflow.Ellipsis,
                            )
                        }
                    }

                    Spacer(modifier = Modifier.height(4.dp))
                    Text(
                        text = "seq:${entry.seq}  hash:${entry.hash.take(12)}...",
                        style = MaterialTheme.typography.labelSmall,
                        fontFamily = FontFamily.Monospace,
                        color = PantheonTextSecondary.copy(alpha = 0.6f),
                    )
                }
            }
        }
    }
}

@Composable
private fun HashChainStatusCard(result: SteleVerifyResult) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = PantheonSurface),
        shape = RoundedCornerShape(12.dp),
    ) {
        Column(modifier = Modifier.padding(16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    text = if (result.isVerified) "\u2705" else "\u274C",
                    fontSize = 24.sp,
                )
                Spacer(modifier = Modifier.width(8.dp))
                Column {
                    Text(
                        text = "Hash Chain: ${result.status.replaceFirstChar { it.uppercase() }}",
                        style = MaterialTheme.typography.titleMedium,
                        color = if (result.isVerified) PantheonSuccess else PantheonError,
                    )
                    Text(
                        text = "${result.chainLength} of ${result.totalCount} entries verified",
                        style = MaterialTheme.typography.bodySmall,
                        color = PantheonTextSecondary,
                    )
                }
            }

            if (result.breaks.isNotEmpty()) {
                Spacer(modifier = Modifier.height(8.dp))
                Text(
                    text = "Chain Breaks:",
                    style = MaterialTheme.typography.labelMedium,
                    fontWeight = FontWeight.Bold,
                    color = PantheonError,
                )
                result.breaks.take(5).forEach { breakMsg ->
                    Text(
                        text = breakMsg,
                        style = MaterialTheme.typography.labelSmall,
                        fontFamily = FontFamily.Monospace,
                        color = PantheonTextSecondary,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
                if (result.breaks.size > 5) {
                    Text(
                        text = "... and ${result.breaks.size - 5} more",
                        style = MaterialTheme.typography.labelSmall,
                        color = PantheonTextSecondary,
                    )
                }
            }
        }
    }
}
