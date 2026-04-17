// 𓃣 Pantheon VS Code Extension — extension.ts
//
// Entry point. Activates on workspace open (onStartupFinished).
// Starts Guardian (always-on renice), status bar ankh, Thoth provider,
// Thoth Accountability Engine, and registers all command palette entries.
//
// Architecture:
//   activate() → starts Guardian background loop
//             → creates status bar ankh with live metrics
//             → registers Command Palette commands
//             → loads Thoth context from .thoth/memory.yaml
//             → runs Thoth Accountability Engine (cold-start benchmark)
//
// The Anubis Suite operates without oversight.

import * as vscode from 'vscode';
import { Guardian } from './guardian';
import { PantheonStatusBar } from './statusBar';
import { registerCommands } from './commands';
import { ThothProvider } from './thothProvider';
import { ThothAccountabilityEngine } from './thothAccountability';
import { CrashpadMonitor } from './crashpadMonitor';

let guardian: Guardian | undefined;
let statusBar: PantheonStatusBar | undefined;
let thothProvider: ThothProvider | undefined;
let accountabilityEngine: ThothAccountabilityEngine | undefined;
let crashpadMonitor: CrashpadMonitor | undefined;

export function activate(context: vscode.ExtensionContext): void {
    const outputChannel = vscode.window.createOutputChannel('Pantheon');
    outputChannel.appendLine('𓃣 Pantheon extension activating...');

    // ── Resolve binary path ───────────────────────────────────────────
    const config = vscode.workspace.getConfiguration('sirsi');
    const binaryPath = config.get<string>('binaryPath', 'sirsi');

    // ── Status Bar (Ankh) ─────────────────────────────────────────────
    statusBar = new PantheonStatusBar(binaryPath, outputChannel);
    context.subscriptions.push(statusBar);

    // ── Guardian (Always-On) ──────────────────────────────────────────
    const guardianEnabled = config.get<boolean>('guardian.enabled', true);
    if (guardianEnabled) {
        const reniceDelay = config.get<number>('guardian.reniceDelay', 30);
        const pollInterval = config.get<number>('guardian.pollInterval', 5);
        const autoRenice = config.get<boolean>('guardian.autoRenice', true);

        guardian = new Guardian(binaryPath, outputChannel, {
            reniceDelaySec: reniceDelay,
            pollIntervalSec: pollInterval,
            autoRenice,
        });
        guardian.start();
        context.subscriptions.push(guardian);

        outputChannel.appendLine(`𓁵 Guardian armed — renice in ${reniceDelay}s, poll every ${pollInterval}s`);
    } else {
        outputChannel.appendLine('𓁵 Guardian disabled by configuration');
    }

    // ── Thoth Context Provider ────────────────────────────────────────
    const thothEnabled = config.get<boolean>('thoth.enabled', true);
    if (thothEnabled) {
        thothProvider = new ThothProvider(outputChannel);
        thothProvider.load();
        context.subscriptions.push(thothProvider);
        outputChannel.appendLine('𓁟 Thoth context provider loaded');
    }

    // ── Thoth Accountability Engine ───────────────────────────────────
    const accountabilityEnabled = config.get<boolean>('thoth.accountability', true);
    if (accountabilityEnabled) {
        accountabilityEngine = new ThothAccountabilityEngine(context, outputChannel);
        context.subscriptions.push(accountabilityEngine);
        // Run benchmark async — don't block activation
        accountabilityEngine.activate().catch(err => {
            const msg = err instanceof Error ? err.message : String(err);
            outputChannel.appendLine(`𓁟 Accountability Engine error: ${msg}`);
        });
        outputChannel.appendLine('𓁟 Thoth Accountability Engine armed');
    }

    // ── Crashpad Monitor ──────────────────────────────────────────────
    crashpadMonitor = new CrashpadMonitor(outputChannel);
    crashpadMonitor.start();
    context.subscriptions.push(crashpadMonitor);
    outputChannel.appendLine('𓁵 Crashpad Monitor armed — tracking IDE stability');

    // ── Command Palette Registration ──────────────────────────────────
    registerCommands(context, binaryPath, outputChannel, statusBar, thothProvider, guardian, accountabilityEngine, crashpadMonitor);

    // ── Workspace Optimization ────────────────────────────────────────
    const autoOptimize = config.get<boolean>('workspace.autoOptimize', false);
    if (autoOptimize) {
        applyOptimalSettings(outputChannel);
    }

    // ── Start metric refresh loop ─────────────────────────────────────
    const pollInterval = config.get<number>('guardian.pollInterval', 5);
    statusBar.startMetricLoop(pollInterval * 1000);

    outputChannel.appendLine('𓃣 Pantheon extension activated — the Anubis Suite is operational');

    // Show welcome notification on first install
    const hasShownWelcome = context.globalState.get<boolean>('pantheon.welcomeShown');
    if (!hasShownWelcome) {
        vscode.window.showInformationMessage(
            '𓃣 Pantheon activated. Guardian is monitoring your workspace.',
            'Show Metrics',
            'Dismiss'
        ).then(choice => {
            if (choice === 'Show Metrics') {
                vscode.commands.executeCommand('sirsi.showMetrics');
            }
        });
        context.globalState.update('pantheon.welcomeShown', true);
    }
}

export function deactivate(): void {
    guardian?.dispose();
    statusBar?.dispose();
    thothProvider?.dispose();
    accountabilityEngine?.dispose();
    crashpadMonitor?.dispose();
}

// ── Workspace Settings ────────────────────────────────────────────────

function applyOptimalSettings(outputChannel: vscode.OutputChannel): void {
    const wsConfig = vscode.workspace.getConfiguration();

    // gopls directory filters — exclude non-Go directories from analysis
    const goplsFilters = wsConfig.get<string[]>('gopls.directoryFilters');
    if (!goplsFilters || goplsFilters.length === 0) {
        wsConfig.update('gopls.directoryFilters', [
            '-**/node_modules',
            '-**/.git',
            '-**/vendor',
            '-**/.vscode-test',
            '-**/dist',
        ], vscode.ConfigurationTarget.Workspace);
        outputChannel.appendLine('𓃣 Applied gopls directory filters');
    }

    // File watcher exclusions — reduce inotify/kqueue pressure
    const existingExcludes = wsConfig.get<Record<string, boolean>>('files.watcherExclude') || {};
    const extraExcludes: Record<string, boolean> = {
        '**/node_modules/**': true,
        '**/.git/objects/**': true,
        '**/.git/subtree-cache/**': true,
        '**/dist/**': true,
        '**/coverage/**': true,
    };

    let needsUpdate = false;
    for (const [pattern, value] of Object.entries(extraExcludes)) {
        if (!(pattern in existingExcludes)) {
            existingExcludes[pattern] = value;
            needsUpdate = true;
        }
    }

    if (needsUpdate) {
        wsConfig.update('files.watcherExclude', existingExcludes, vscode.ConfigurationTarget.Workspace);
        outputChannel.appendLine('𓃣 Applied file watcher exclusions');
    }

    // Disable shell integration if causing issues
    const shellIntegration = wsConfig.get<boolean>('terminal.integrated.shellIntegration.enabled');
    if (shellIntegration !== false) {
        wsConfig.update('terminal.integrated.shellIntegration.enabled', false, vscode.ConfigurationTarget.Workspace);
        outputChannel.appendLine('𓃣 Disabled shell integration (reduces Extension Host CPU)');
    }
}
