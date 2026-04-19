package ai.sirsi.pantheon.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import ai.sirsi.pantheon.R
import ai.sirsi.pantheon.bridge.PantheonBridge
import ai.sirsi.pantheon.models.ProjectInfo
import ai.sirsi.pantheon.ui.components.DeityHeader
import ai.sirsi.pantheon.ui.theme.PantheonError
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary

/**
 * Thoth memory management screen. Allows syncing and compacting
 * project knowledge stores.
 */
@Composable
fun ThothScreen() {
    var isSyncing by remember { mutableStateOf(false) }
    var projectInfo by remember { mutableStateOf<ProjectInfo?>(null) }
    var syncStatus by remember { mutableStateOf<String?>(null) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        DeityHeader(
            glyph = "\uD80C\uDC5F",
            name = stringResource(R.string.thoth_name),
            subtitle = stringResource(R.string.thoth_subtitle),
            description = stringResource(R.string.thoth_description),
        )

        Button(
            onClick = {
                scope.launch {
                    isSyncing = true
                    errorMessage = null
                    try {
                        val dataDir = PantheonBridge.version() // verify bridge
                        val root = android.os.Environment.getExternalStorageDirectory().absolutePath
                        projectInfo = PantheonBridge.thothDetectProject(root)
                        PantheonBridge.thothSync(root)
                        syncStatus = "Synced"
                    } catch (e: Exception) {
                        errorMessage = e.message ?: stringResource(R.string.error_generic)
                    } finally {
                        isSyncing = false
                    }
                }
            },
            enabled = !isSyncing,
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.buttonColors(containerColor = PantheonGold),
            shape = RoundedCornerShape(8.dp),
        ) {
            if (isSyncing) {
                CircularProgressIndicator(
                    color = MaterialTheme.colorScheme.onPrimary,
                    strokeWidth = 2.dp,
                )
            } else {
                Text(stringResource(R.string.action_sync))
            }
        }

        // Error
        if (errorMessage != null) {
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

        // Project Info
        if (projectInfo != null) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                shape = RoundedCornerShape(12.dp),
            ) {
                Column(modifier = Modifier.padding(16.dp)) {
                    Text(
                        text = "Project Info",
                        style = MaterialTheme.typography.titleMedium,
                        color = PantheonGold,
                    )
                    Spacer(modifier = Modifier.height(8.dp))
                    InfoRow("Name", projectInfo?.name ?: "Unknown")
                    InfoRow("Language", projectInfo?.language ?: "Unknown")
                    InfoRow("Version", projectInfo?.version ?: "Unknown")
                    InfoRow("Root", projectInfo?.root ?: "N/A")
                }
            }
        }

        // Sync status
        if (syncStatus != null) {
            Card(
                modifier = Modifier.fillMaxWidth(),
                colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                shape = RoundedCornerShape(12.dp),
            ) {
                Text(
                    text = "Status: $syncStatus",
                    modifier = Modifier.padding(16.dp),
                    style = MaterialTheme.typography.bodyLarge,
                    color = PantheonGold,
                )
            }
        }
    }
}

@Composable
internal fun InfoRow(label: String, value: String) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 4.dp),
    ) {
        Text(
            text = label,
            style = MaterialTheme.typography.bodyMedium,
            color = PantheonTextSecondary,
            modifier = Modifier.weight(0.35f),
        )
        Text(
            text = value,
            style = MaterialTheme.typography.bodyMedium,
            modifier = Modifier.weight(0.65f),
        )
    }
}
