package ai.sirsi.pantheon

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Radar
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import ai.sirsi.pantheon.ui.screens.AnubisScreen
import ai.sirsi.pantheon.ui.screens.HomeScreen
import ai.sirsi.pantheon.ui.screens.KaScreen
import ai.sirsi.pantheon.ui.screens.SebaScreen
import ai.sirsi.pantheon.ui.screens.ThothScreen
import ai.sirsi.pantheon.ui.theme.PantheonTheme

/**
 * Main entry point for the Pantheon Android app.
 * Uses Jetpack Compose with bottom navigation across deity screens.
 */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            PantheonTheme {
                PantheonNavHost()
            }
        }
    }
}

/** Navigation route constants. */
object Routes {
    const val HOME = "home"
    const val ANUBIS = "anubis"
    const val KA = "ka"
    const val THOTH = "thoth"
    const val SEBA = "seba"
}

/** Bottom navigation item definition. */
data class NavItem(
    val route: String,
    val labelRes: Int,
    val icon: androidx.compose.ui.graphics.vector.ImageVector,
)

@Composable
fun PantheonNavHost() {
    val navController = rememberNavController()

    val navItems = listOf(
        NavItem(Routes.HOME, R.string.nav_home, Icons.Default.Home),
        NavItem(Routes.ANUBIS, R.string.nav_anubis, Icons.Default.Search),
        NavItem(Routes.KA, R.string.nav_ka, Icons.Default.Visibility),
        NavItem(Routes.THOTH, R.string.nav_thoth, Icons.Default.Memory),
        NavItem(Routes.SEBA, R.string.nav_seba, Icons.Default.Radar),
    )

    Scaffold(
        modifier = Modifier.fillMaxSize(),
        bottomBar = {
            val navBackStackEntry by navController.currentBackStackEntryAsState()
            val currentDestination = navBackStackEntry?.destination

            NavigationBar(
                containerColor = ai.sirsi.pantheon.ui.theme.PantheonBlack,
            ) {
                navItems.forEach { item ->
                    val selected = currentDestination?.hierarchy?.any { it.route == item.route } == true
                    NavigationBarItem(
                        icon = { Icon(item.icon, contentDescription = stringResource(item.labelRes)) },
                        label = { Text(stringResource(item.labelRes)) },
                        selected = selected,
                        onClick = {
                            navController.navigate(item.route) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    saveState = true
                                }
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                        colors = NavigationBarItemDefaults.colors(
                            selectedIconColor = ai.sirsi.pantheon.ui.theme.PantheonGold,
                            selectedTextColor = ai.sirsi.pantheon.ui.theme.PantheonGold,
                            unselectedIconColor = ai.sirsi.pantheon.ui.theme.PantheonTextSecondary,
                            unselectedTextColor = ai.sirsi.pantheon.ui.theme.PantheonTextSecondary,
                            indicatorColor = ai.sirsi.pantheon.ui.theme.PantheonSurface,
                        ),
                    )
                }
            }
        },
    ) { innerPadding ->
        NavHost(
            navController = navController,
            startDestination = Routes.HOME,
            modifier = Modifier.padding(innerPadding),
        ) {
            composable(Routes.HOME) { HomeScreen() }
            composable(Routes.ANUBIS) { AnubisScreen() }
            composable(Routes.KA) { KaScreen() }
            composable(Routes.THOTH) { ThothScreen() }
            composable(Routes.SEBA) { SebaScreen() }
        }
    }
}
