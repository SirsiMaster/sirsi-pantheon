package ai.sirsi.pantheon.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

// Pantheon Brand Colors
val PantheonGold = Color(0xFFC8A951)
val PantheonBlack = Color(0xFF0F0F0F)
val PantheonLapis = Color(0xFF1A1A5E)
val PantheonSurface = Color(0xFF1A1A1A)
val PantheonError = Color(0xFFCF6679)
val PantheonSuccess = Color(0xFF4CAF50)
val PantheonTextPrimary = Color(0xFFFFFFFF)
val PantheonTextSecondary = Color(0xFFB0B0B0)

private val PantheonColorScheme = darkColorScheme(
    primary = PantheonGold,
    onPrimary = PantheonBlack,
    secondary = PantheonLapis,
    onSecondary = Color.White,
    tertiary = PantheonLapis,
    background = PantheonBlack,
    onBackground = PantheonTextPrimary,
    surface = PantheonSurface,
    onSurface = PantheonTextPrimary,
    surfaceVariant = PantheonSurface,
    onSurfaceVariant = PantheonTextSecondary,
    error = PantheonError,
    onError = PantheonBlack,
)

/**
 * Pantheon Material 3 theme.
 * Always dark — matches the Egyptian gold-on-black brand identity.
 */
@Composable
fun PantheonTheme(
    content: @Composable () -> Unit,
) {
    MaterialTheme(
        colorScheme = PantheonColorScheme,
        typography = PantheonTypography,
        content = content,
    )
}
