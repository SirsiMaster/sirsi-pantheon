// 𓁆 Seshat — Gemini Bridge VS Code Extension
//
// Entry point for the Seshat Knowledge Bridge extension.
// Provides bidirectional sync between Gemini AI Mode, NotebookLM,
// and Antigravity IDE via tree views and command palette actions.
//
// Architecture:
//   activate() → registers tree view providers for KIs, profiles, sync status
//             → registers all command palette commands
//             → optionally triggers auto-sync on startup
//             → creates status bar indicator

import * as vscode from 'vscode';
import { registerCommands } from './commands';
import { KnowledgeItemsProvider } from './knowledgeProvider';
import { ChromeProfilesProvider } from './chromeProfilesProvider';
import { SyncStatusProvider } from './syncStatusProvider';
import { SeshatDashboard } from './dashboard';
import { resolvePaths } from './paths';

let knowledgeProvider: KnowledgeItemsProvider | undefined;
let profilesProvider: ChromeProfilesProvider | undefined;
let syncProvider: SyncStatusProvider | undefined;
let statusBarItem: vscode.StatusBarItem | undefined;

export function activate(context: vscode.ExtensionContext): void {
    const outputChannel = vscode.window.createOutputChannel('Seshat Bridge');
    outputChannel.appendLine('𓁆 Seshat — Gemini Bridge activating...');

    const config = vscode.workspace.getConfiguration('seshat');
    const paths = resolvePaths(config.get<string>('antigravityDir', ''));

    // ── Status Bar ────────────────────────────────────────────────────
    statusBarItem = vscode.window.createStatusBarItem(
        vscode.StatusBarAlignment.Right,
        90
    );
    statusBarItem.text = '𓁆 Seshat';
    statusBarItem.tooltip = 'Gemini Bridge — Click to open dashboard';
    statusBarItem.command = 'seshat.showDashboard';
    statusBarItem.show();
    context.subscriptions.push(statusBarItem);

    // ── Tree View Providers ───────────────────────────────────────────
    knowledgeProvider = new KnowledgeItemsProvider(paths, outputChannel);
    profilesProvider = new ChromeProfilesProvider(outputChannel);
    syncProvider = new SyncStatusProvider(paths, outputChannel);

    context.subscriptions.push(
        vscode.window.registerTreeDataProvider('seshat.knowledgeItems', knowledgeProvider),
        vscode.window.registerTreeDataProvider('seshat.chromeProfiles', profilesProvider),
        vscode.window.registerTreeDataProvider('seshat.syncStatus', syncProvider)
    );

    // ── Dashboard Panel ───────────────────────────────────────────────
    const dashboard = new SeshatDashboard(context, paths, outputChannel);

    // ── Command Registration ──────────────────────────────────────────
    registerCommands(
        context,
        paths,
        outputChannel,
        knowledgeProvider,
        profilesProvider,
        syncProvider,
        dashboard
    );

    // ── Auto-Sync ─────────────────────────────────────────────────────
    const autoSync = config.get<boolean>('autoSync', false);
    if (autoSync) {
        outputChannel.appendLine('𓁆 Auto-sync enabled — syncing KIs on startup...');
        vscode.commands.executeCommand('seshat.syncToGemini').then(
            () => outputChannel.appendLine('𓁆 Auto-sync complete'),
            (err: Error) => outputChannel.appendLine(`𓁆 Auto-sync error: ${err.message}`)
        );
    }

    outputChannel.appendLine('𓁆 Seshat — Gemini Bridge activated. Six directions online.');

    // Show welcome on first install
    const hasShownWelcome = context.globalState.get<boolean>('seshat.welcomeShown');
    if (!hasShownWelcome) {
        vscode.window.showInformationMessage(
            '𓁆 Seshat activated. Gemini Bridge is ready to sync knowledge.',
            'Open Dashboard',
            'Dismiss'
        ).then(choice => {
            if (choice === 'Open Dashboard') {
                vscode.commands.executeCommand('seshat.showDashboard');
            }
        });
        context.globalState.update('seshat.welcomeShown', true);
    }
}

export function deactivate(): void {
    statusBarItem?.dispose();
}
