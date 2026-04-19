package ai.sirsi.pantheon.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import ai.sirsi.pantheon.R
import ai.sirsi.pantheon.bridge.PantheonBridge
import ai.sirsi.pantheon.ui.components.DeityCard
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary

/**
 * Home dashboard screen displaying the Pantheon header and deity cards.
 */
@Composable
fun HomeScreen(
    onNavigate: ((String) -> Unit)? = null,
) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 16.dp, vertical = 24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        // Header
        Text(
            text = "\uD80C\uDC1F", // Thoth hieroglyph
            fontSize = 48.sp,
        )
        Spacer(modifier = Modifier.height(8.dp))
        Text(
            text = stringResource(R.string.home_title),
            style = MaterialTheme.typography.displayLarge,
            color = PantheonGold,
        )
        Text(
            text = stringResource(R.string.home_subtitle),
            style = MaterialTheme.typography.bodyLarge,
            color = PantheonTextSecondary,
        )
        Text(
            text = stringResource(R.string.home_motto),
            style = MaterialTheme.typography.bodyMedium,
            fontStyle = FontStyle.Italic,
            color = PantheonTextSecondary,
        )

        Spacer(modifier = Modifier.height(8.dp))

        Text(
            text = "v${PantheonBridge.version()}",
            style = MaterialTheme.typography.bodySmall,
            color = PantheonTextSecondary,
        )

        Spacer(modifier = Modifier.height(24.dp))

        // Deity Cards
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            DeityCard(
                glyph = "\uD80C\uDCE3", // Anubis jackal
                name = stringResource(R.string.anubis_name),
                subtitle = stringResource(R.string.anubis_subtitle),
                description = stringResource(R.string.anubis_description),
                onClick = { onNavigate?.invoke("anubis") },
            )
            DeityCard(
                glyph = "\uD80C\uDC93", // Ka spirit
                name = stringResource(R.string.ka_name),
                subtitle = stringResource(R.string.ka_subtitle),
                description = stringResource(R.string.ka_description),
                onClick = { onNavigate?.invoke("ka") },
            )
            DeityCard(
                glyph = "\uD80C\uDC5F", // Thoth ibis
                name = stringResource(R.string.thoth_name),
                subtitle = stringResource(R.string.thoth_subtitle),
                description = stringResource(R.string.thoth_description),
                onClick = { onNavigate?.invoke("thoth") },
            )
            DeityCard(
                glyph = "\uD80C\uDFBC", // Seba star
                name = stringResource(R.string.seba_name),
                subtitle = stringResource(R.string.seba_subtitle),
                description = stringResource(R.string.seba_description),
                onClick = { onNavigate?.invoke("seba") },
            )
            DeityCard(
                glyph = "\uD80C\uDC80", // Seshat
                name = stringResource(R.string.seshat_name),
                subtitle = stringResource(R.string.seshat_subtitle),
                description = stringResource(R.string.seshat_description),
                onClick = { /* Seshat screen not in bottom nav — future deep link */ },
            )
        }

        Spacer(modifier = Modifier.height(16.dp))
    }
}
