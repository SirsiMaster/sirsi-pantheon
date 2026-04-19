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
import ai.sirsi.pantheon.models.GhostApp
import ai.sirsi.pantheon.ui.components.DeityHeader
import ai.sirsi.pantheon.ui.components.StatPill
import ai.sirsi.pantheon.ui.theme.PantheonError
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary

/**
 * Ka ghost detection screen. Hunts for residual files left behind
 * by uninstalled applications.
 */
@Composable
fun KaScreen() {
    var isHunting by remember { mutableStateOf(false) }
    var ghosts by remember { mutableStateOf<List<GhostApp>?>(null) }
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
                glyph = "\uD80C\uDC93",
                name = stringResource(R.string.ka_name),
                subtitle = stringResource(R.string.ka_subtitle),
                description = stringResource(R.string.ka_description),
            )
        }

        item {
            Button(
                onClick = {
                    scope.launch {
                        isHunting = true
                        errorMessage = null
                        try {
                            ghosts = PantheonBridge.kaHunt(includeSudo = false)
                        } catch (e: Exception) {
                            errorMessage = e.message ?: stringResource(R.string.error_generic)
                        } finally {
                            isHunting = false
                        }
                    }
                },
                enabled = !isHunting,
                modifier = Modifier.fillMaxWidth(),
                colors = ButtonDefaults.buttonColors(containerColor = PantheonGold),
                shape = RoundedCornerShape(8.dp),
            ) {
                if (isHunting) {
                    CircularProgressIndicator(
                        color = MaterialTheme.colorScheme.onPrimary,
                        strokeWidth = 2.dp,
                    )
                } else {
                    Text(stringResource(R.string.action_hunt))
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
        if (ghosts != null) {
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
                                label = stringResource(R.string.label_ghosts),
                                value = "${ghosts!!.size}",
                            )
                            StatPill(
                                label = stringResource(R.string.label_residuals),
                                value = "${ghosts!!.sumOf { it.residuals.size }}",
                            )
                            StatPill(
                                label = stringResource(R.string.label_total_size),
                                value = ai.sirsi.pantheon.models.formatBytes(
                                    ghosts!!.sumOf { it.totalSize }
                                ),
                            )
                        }
                    }
                }
            }
        }

        // Ghost list
        val ghostList = ghosts.orEmpty()
        if (ghostList.isEmpty() && ghosts != null) {
            item {
                Text(
                    text = stringResource(R.string.label_no_ghosts),
                    style = MaterialTheme.typography.bodyMedium,
                    color = PantheonTextSecondary,
                    modifier = Modifier.padding(vertical = 8.dp),
                )
            }
        }

        items(ghostList, key = { it.id }) { ghost ->
            GhostRow(ghost)
        }
    }
}

@Composable
private fun GhostRow(ghost: GhostApp) {
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
                    text = ghost.appName,
                    style = MaterialTheme.typography.bodyLarge,
                )
                Text(
                    text = "${ghost.residuals.size} residuals \u2022 ${ghost.totalFiles} files",
                    style = MaterialTheme.typography.bodySmall,
                    color = PantheonTextSecondary,
                )
            }
            Text(
                text = ghost.formattedSize,
                style = MaterialTheme.typography.titleMedium,
                color = PantheonGold,
            )
        }
    }
}
