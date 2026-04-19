package ai.sirsi.pantheon.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import ai.sirsi.pantheon.R
import ai.sirsi.pantheon.bridge.PantheonBridge
import ai.sirsi.pantheon.models.Finding
import ai.sirsi.pantheon.models.ScanResult
import ai.sirsi.pantheon.ui.components.DeityHeader
import ai.sirsi.pantheon.ui.components.StatPill
import ai.sirsi.pantheon.ui.theme.PantheonError
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary

/**
 * Anubis scan screen. Allows the user to run infrastructure scans
 * and view findings with reclaimable storage details.
 */
@Composable
fun AnubisScreen() {
    var isScanning by remember { mutableStateOf(false) }
    var scanResult by remember { mutableStateOf<ScanResult?>(null) }
    var errorMessage by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 16.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            DeityHeader(
                glyph = "\uD80C\uDCE3",
                name = stringResource(R.string.anubis_name),
                subtitle = stringResource(R.string.anubis_subtitle),
                description = stringResource(R.string.anubis_description),
            )
        }

        item {
            Button(
                onClick = {
                    scope.launch {
                        isScanning = true
                        errorMessage = null
                        try {
                            val dataDir = PantheonBridge.version() // verify bridge works
                            scanResult = PantheonBridge.anubisScan(
                                rootPath = android.os.Environment.getExternalStorageDirectory().absolutePath,
                            )
                        } catch (e: Exception) {
                            errorMessage = e.message ?: stringResource(R.string.error_scan_failed)
                        } finally {
                            isScanning = false
                        }
                    }
                },
                enabled = !isScanning,
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(containerColor = PantheonGold),
                shape = RoundedCornerShape(8.dp),
            ) {
                if (isScanning) {
                    CircularProgressIndicator(
                        color = MaterialTheme.colorScheme.onPrimary,
                        strokeWidth = 2.dp,
                    )
                } else {
                    Text(stringResource(R.string.action_scan))
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

        // Results summary
        if (scanResult != null) {
            item {
                Card(
                    modifier = Modifier.fillMaxWidth(),
                    colors = CardDefaults.cardColors(containerColor = PantheonSurface),
                    shape = RoundedCornerShape(12.dp),
                ) {
                    Column(modifier = Modifier.padding(16.dp)) {
                        Text(
                            text = stringResource(R.string.state_complete),
                            style = MaterialTheme.typography.titleMedium,
                        )
                        Spacer(modifier = Modifier.height(12.dp))
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceEvenly,
                        ) {
                            StatPill(
                                label = stringResource(R.string.label_findings),
                                value = "${scanResult!!.findings.size}",
                            )
                            StatPill(
                                label = stringResource(R.string.label_reclaimable),
                                value = scanResult!!.formattedSize,
                            )
                            StatPill(
                                label = stringResource(R.string.label_rules_ran),
                                value = "${scanResult!!.rulesRan}",
                            )
                        }
                    }
                }
            }
        }

        // Findings list
        val findings = scanResult?.findings.orEmpty()
        if (findings.isEmpty() && scanResult != null) {
            item {
                Text(
                    text = stringResource(R.string.label_no_findings),
                    style = MaterialTheme.typography.bodyMedium,
                    color = PantheonTextSecondary,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
            }
        }

        items(findings, key = { it.id }) { finding ->
            FindingRow(finding)
        }
    }
}

@Composable
private fun FindingRow(finding: Finding) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = PantheonSurface),
        shape = RoundedCornerShape(8.dp),
    ) {
        Row(
            modifier = Modifier.padding(12.dp),
            verticalAlignment = Alignment.Top,
        ) {
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = finding.description,
                    style = MaterialTheme.typography.bodyLarge,
                )
                Text(
                    text = finding.path,
                    style = MaterialTheme.typography.bodySmall,
                    color = PantheonTextSecondary,
                    maxLines = 1,
                )
            }
            Text(
                text = finding.formattedSize,
                style = MaterialTheme.typography.titleMedium,
                color = PantheonGold,
            )
        }
    }
}
