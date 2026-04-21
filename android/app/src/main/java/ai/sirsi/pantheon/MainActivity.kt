package ai.sirsi.pantheon

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AccountTree
import androidx.compose.material.icons.filled.FilterAlt
import androidx.compose.material.icons.filled.FormatListBulleted
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Memory
import androidx.compose.material.icons.filled.Menu
import androidx.compose.material.icons.filled.Psychology
import androidx.compose.material.icons.filled.Radar
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.Warehouse
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalDrawerSheet
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.NavigationDrawerItem
import androidx.compose.material3.NavigationDrawerItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.NavGraph.Companion.findStartDestination
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.launch
import ai.sirsi.pantheon.ui.screens.AnubisScreen
import ai.sirsi.pantheon.ui.screens.BrainScreen
import ai.sirsi.pantheon.ui.screens.HomeScreen
import ai.sirsi.pantheon.ui.screens.HorusScreen
import ai.sirsi.pantheon.ui.screens.KaScreen
import ai.sirsi.pantheon.ui.screens.RTKScreen
import ai.sirsi.pantheon.ui.screens.SebaScreen
import ai.sirsi.pantheon.ui.screens.ThothScreen
import ai.sirsi.pantheon.ui.screens.SteleScreen
import ai.sirsi.pantheon.ui.screens.VaultScreen
import ai.sirsi.pantheon.ui.theme.PantheonBlack
import ai.sirsi.pantheon.ui.theme.PantheonGold
import ai.sirsi.pantheon.ui.theme.PantheonSurface
import ai.sirsi.pantheon.ui.theme.PantheonTextSecondary
import ai.sirsi.pantheon.ui.theme.PantheonTheme

/**
 * Main entry point for the Pantheon Android app.
 * Uses Jetpack Compose with a navigation drawer for all deity screens.
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
    const val RTK = "rtk"
    const val VAULT = "vault"
    const val HORUS = "horus"
    const val BRAIN = "brain"
    const val STELE = "stele"
}

/** Navigation drawer item definition. */
data class NavItem(
    val route: String,
    val labelRes: Int,
    val icon: androidx.compose.ui.graphics.vector.ImageVector,
    val section: String = "core",
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PantheonNavHost() {
    val navController = rememberNavController()
    val drawerState = rememberDrawerState(initialValue = DrawerValue.Closed)
    val scope = rememberCoroutineScope()

    val coreItems = listOf(
        NavItem(Routes.HOME, R.string.nav_home, Icons.Default.Home),
        NavItem(Routes.ANUBIS, R.string.nav_anubis, Icons.Default.Search),
        NavItem(Routes.KA, R.string.nav_ka, Icons.Default.Visibility),
        NavItem(Routes.THOTH, R.string.nav_thoth, Icons.Default.Memory),
        NavItem(Routes.SEBA, R.string.nav_seba, Icons.Default.Radar),
    )

    val advancedItems = listOf(
        NavItem(Routes.RTK, R.string.nav_rtk, Icons.Default.FilterAlt, "advanced"),
        NavItem(Routes.VAULT, R.string.nav_vault, Icons.Default.Warehouse, "advanced"),
        NavItem(Routes.HORUS, R.string.nav_horus, Icons.Default.AccountTree, "advanced"),
        NavItem(Routes.BRAIN, R.string.nav_brain, Icons.Default.Psychology, "advanced"),
        NavItem(Routes.STELE, R.string.nav_stele, Icons.Default.FormatListBulleted, "advanced"),
    )

    ModalNavigationDrawer(
        drawerState = drawerState,
        drawerContent = {
            ModalDrawerSheet(
                drawerContainerColor = PantheonBlack,
            ) {
                Spacer(modifier = Modifier.height(24.dp))
                Text(
                    text = stringResource(R.string.home_title),
                    style = androidx.compose.material3.MaterialTheme.typography.headlineMedium,
                    color = PantheonGold,
                    modifier = Modifier.padding(horizontal = 28.dp, vertical = 8.dp),
                )
                Text(
                    text = stringResource(R.string.home_subtitle),
                    style = androidx.compose.material3.MaterialTheme.typography.bodySmall,
                    color = PantheonTextSecondary,
                    modifier = Modifier.padding(horizontal = 28.dp, vertical = 0.dp),
                )
                Spacer(modifier = Modifier.height(16.dp))

                val navBackStackEntry by navController.currentBackStackEntryAsState()
                val currentDestination = navBackStackEntry?.destination

                // Core deity items
                coreItems.forEach { item ->
                    val selected = currentDestination?.hierarchy?.any { it.route == item.route } == true
                    NavigationDrawerItem(
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
                            scope.launch { drawerState.close() }
                        },
                        colors = NavigationDrawerItemDefaults.colors(
                            selectedContainerColor = PantheonSurface,
                            selectedIconColor = PantheonGold,
                            selectedTextColor = PantheonGold,
                            unselectedIconColor = PantheonTextSecondary,
                            unselectedTextColor = PantheonTextSecondary,
                        ),
                        modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
                    )
                }

                // Divider between core and advanced
                HorizontalDivider(
                    modifier = Modifier.padding(horizontal = 28.dp, vertical = 12.dp),
                    color = PantheonSurface,
                )

                Text(
                    text = "Advanced",
                    style = androidx.compose.material3.MaterialTheme.typography.labelMedium,
                    color = PantheonTextSecondary,
                    modifier = Modifier.padding(horizontal = 28.dp, vertical = 4.dp),
                )

                // Advanced deity items
                advancedItems.forEach { item ->
                    val selected = currentDestination?.hierarchy?.any { it.route == item.route } == true
                    NavigationDrawerItem(
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
                            scope.launch { drawerState.close() }
                        },
                        colors = NavigationDrawerItemDefaults.colors(
                            selectedContainerColor = PantheonSurface,
                            selectedIconColor = PantheonGold,
                            selectedTextColor = PantheonGold,
                            unselectedIconColor = PantheonTextSecondary,
                            unselectedTextColor = PantheonTextSecondary,
                        ),
                        modifier = Modifier.padding(NavigationDrawerItemDefaults.ItemPadding),
                    )
                }
            }
        },
    ) {
        Scaffold(
            modifier = Modifier.fillMaxSize(),
            topBar = {
                TopAppBar(
                    title = {
                        val navBackStackEntry by navController.currentBackStackEntryAsState()
                        val currentRoute = navBackStackEntry?.destination?.route ?: Routes.HOME
                        val titleRes = when (currentRoute) {
                            Routes.HOME -> R.string.home_title
                            Routes.ANUBIS -> R.string.anubis_name
                            Routes.KA -> R.string.ka_name
                            Routes.THOTH -> R.string.thoth_name
                            Routes.SEBA -> R.string.seba_name
                            Routes.RTK -> R.string.rtk_name
                            Routes.VAULT -> R.string.vault_name
                            Routes.HORUS -> R.string.horus_name
                            Routes.BRAIN -> R.string.brain_name
                            Routes.STELE -> R.string.stele_name
                            else -> R.string.home_title
                        }
                        Text(
                            text = stringResource(titleRes),
                            color = PantheonGold,
                        )
                    },
                    navigationIcon = {
                        IconButton(onClick = { scope.launch { drawerState.open() } }) {
                            Icon(
                                Icons.Default.Menu,
                                contentDescription = "Menu",
                                tint = PantheonGold,
                            )
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(
                        containerColor = PantheonBlack,
                    ),
                )
            },
        ) { innerPadding ->
            NavHost(
                navController = navController,
                startDestination = Routes.HOME,
                modifier = Modifier.padding(innerPadding),
            ) {
                composable(Routes.HOME) {
                    HomeScreen(
                        onNavigate = { route ->
                            navController.navigate(route) {
                                popUpTo(navController.graph.findStartDestination().id) {
                                    saveState = true
                                }
                                launchSingleTop = true
                                restoreState = true
                            }
                        },
                    )
                }
                composable(Routes.ANUBIS) { AnubisScreen() }
                composable(Routes.KA) { KaScreen() }
                composable(Routes.THOTH) { ThothScreen() }
                composable(Routes.SEBA) { SebaScreen() }
                composable(Routes.RTK) { RTKScreen() }
                composable(Routes.VAULT) { VaultScreen() }
                composable(Routes.HORUS) { HorusScreen() }
                composable(Routes.BRAIN) { BrainScreen() }
                composable(Routes.STELE) { SteleScreen() }
            }
        }
    }
}
