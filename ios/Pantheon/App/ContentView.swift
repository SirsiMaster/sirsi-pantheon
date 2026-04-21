import SwiftUI

/// Root content view — adapts layout based on device:
/// - iPhone: Tab bar with deity tabs (compact width)
/// - iPad: NavigationSplitView with sidebar + detail (regular width)
struct ContentView: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.horizontalSizeClass) var sizeClass

    var body: some View {
        Group {
            if sizeClass == .regular {
                iPadLayout
            } else {
                iPhoneLayout
            }
        }
        .tint(PantheonTheme.gold)
    }

    // MARK: - iPad: Sidebar + Detail

    private var iPadLayout: some View {
        NavigationSplitView {
            sidebar
                .navigationTitle("Pantheon")
        } detail: {
            deityDetailView
                .navigationTitle(appState.activeDeity.rawValue)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .principal) {
                        deityToolbarTitle
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        modePicker
                    }
                }
        }
        .navigationSplitViewStyle(.balanced)
    }

    private var sidebar: some View {
        List(AppState.ActiveDeity.allCases) { deity in
            Button {
                appState.activeDeity = deity
            } label: {
                HStack(spacing: 12) {
                    Text(deity.glyph)
                        .font(.title2)
                        .frame(width: 32)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(deity.rawValue)
                            .font(.headline)
                            .foregroundStyle(
                                appState.activeDeity == deity
                                    ? PantheonTheme.gold
                                    : PantheonTheme.textPrimary
                            )
                        Text(deity.subtitle)
                            .font(.caption)
                            .foregroundStyle(PantheonTheme.textSecondary)
                    }
                }
                .padding(.vertical, 4)
            }
            .listRowBackground(
                appState.activeDeity == deity
                    ? PantheonTheme.gold.opacity(0.1)
                    : Color.clear
            )
        }
        .listStyle(.sidebar)
        .scrollContentBackground(.hidden)
        .background(PantheonTheme.background)
    }

    // MARK: - iPhone: Tab Bar

    private var iPhoneLayout: some View {
        NavigationStack {
            VStack(spacing: 0) {
                modePicker
                    .padding(.horizontal)
                    .padding(.top, 8)

                Group {
                    switch appState.viewMode {
                    case .gui:
                        GUIContainerView()
                    case .tui:
                        TUIContainerView()
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .background(PantheonTheme.background)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .principal) {
                    deityToolbarTitle
                }
            }
        }
    }

    // MARK: - Shared Components

    private var deityToolbarTitle: some View {
        HStack(spacing: 6) {
            Text(appState.activeDeity.glyph)
                .font(.title2)
            Text(appState.activeDeity.rawValue)
                .font(.headline)
                .foregroundStyle(PantheonTheme.gold)
        }
    }

    private var modePicker: some View {
        Picker("Mode", selection: $appState.viewMode) {
            ForEach(AppState.ViewMode.allCases, id: \.self) { mode in
                Text(mode.rawValue).tag(mode)
            }
        }
        .pickerStyle(.segmented)
    }

    @ViewBuilder
    private var deityDetailView: some View {
        switch appState.viewMode {
        case .gui:
            deityView(for: appState.activeDeity)
        case .tui:
            TUIContainerView()
        }
    }

    @ViewBuilder
    private func deityView(for deity: AppState.ActiveDeity) -> some View {
        switch deity {
        case .anubis: AnubisView()
        case .ka:     KaView()
        case .thoth:  ThothView()
        case .seba:   SebaView()
        case .seshat: SeshatView()
        case .brain:  BrainView()
        case .rtk:    RTKView()
        case .vault:  VaultView()
        case .horus:  HorusView()
        case .stele:  SteleView()
        }
    }
}

/// GUI mode: native SwiftUI views with tab bar for deity selection (iPhone).
struct GUIContainerView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        TabView(selection: $appState.activeDeity) {
            AnubisView()
                .tabItem {
                    Label("Anubis", systemImage: "magnifyingglass.circle.fill")
                }
                .tag(AppState.ActiveDeity.anubis)

            KaView()
                .tabItem {
                    Label("Ka", systemImage: "eye.trianglebadge.exclamationmark.fill")
                }
                .tag(AppState.ActiveDeity.ka)

            ThothView()
                .tabItem {
                    Label("Thoth", systemImage: "brain.head.profile.fill")
                }
                .tag(AppState.ActiveDeity.thoth)

            SebaView()
                .tabItem {
                    Label("Seba", systemImage: "cpu.fill")
                }
                .tag(AppState.ActiveDeity.seba)

            SeshatView()
                .tabItem {
                    Label("Seshat", systemImage: "books.vertical.fill")
                }
                .tag(AppState.ActiveDeity.seshat)

            BrainView()
                .tabItem {
                    Label("Brain", systemImage: "brain.fill")
                }
                .tag(AppState.ActiveDeity.brain)

            RTKView()
                .tabItem {
                    Label("RTK", systemImage: "bolt.fill")
                }
                .tag(AppState.ActiveDeity.rtk)

            VaultView()
                .tabItem {
                    Label("Vault", systemImage: "building.columns.fill")
                }
                .tag(AppState.ActiveDeity.vault)

            HorusView()
                .tabItem {
                    Label("Horus", systemImage: "eye.circle.fill")
                }
                .tag(AppState.ActiveDeity.horus)

            SteleView()
                .tabItem {
                    Label("Stele", systemImage: "list.bullet.rectangle.fill")
                }
                .tag(AppState.ActiveDeity.stele)
        }
    }
}
