// 𓃣 Pantheon Commands — commands.ts
//
// Command Palette registrations for the Pantheon extension.
// All commands are prefixed with "Pantheon:" in the palette.
//
// Commands:
//   sirsi.scan               — Scan workspace via Jackal
//   sirsi.guard              — Start/restart Guardian
//   sirsi.reniceLSP          — Manual renice of LSP processes
//   sirsi.ghostReport        — Ka ghost detection
//   sirsi.thothContext       — Show Thoth compressed context
//   sirsi.showMetrics        — Display system metrics dashboard
//   sirsi.thothAccountability — Full Thoth Accountability Report
//   sirsi.crashpadReport     — Crashpad stability report
//   sirsi.applyWorkspaceSettings — Apply optimal IDE settings

import { execFile } from 'child_process';
import { promisify } from 'util';
import * as vscode from 'vscode';
import { PantheonStatusBar } from './statusBar';
import { ThothProvider } from './thothProvider';
import { Guardian } from './guardian';
import { ThothAccountabilityEngine } from './thothAccountability';
import { CrashpadMonitor } from './crashpadMonitor';

const execFileAsync = promisify(execFile);

export function registerCommands(
    context: vscode.ExtensionContext,
    binaryPath: string,
    output: vscode.OutputChannel,
    statusBar: PantheonStatusBar | undefined,
    thothProvider: ThothProvider | undefined,
    guardian: Guardian | undefined,
    accountabilityEngine: ThothAccountabilityEngine | undefined,
    crashpadMonitor: CrashpadMonitor | undefined
): void {

    // ── Scan Workspace ────────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.scan', async () => {
            const workspaceRoot = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
            if (!workspaceRoot) {
                vscode.window.showWarningMessage('𓃣 Pantheon: No workspace folder open');
                return;
            }

            await vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: '𓃣 Pantheon: Scanning workspace...',
                cancellable: true,
            }, async (progress, token) => {
                try {
                    const { stdout } = await execFileAsync(binaryPath, [
                        'scan', '--json'
                    ], {
                        timeout: 60000,
                    });

                    // Show results in output channel
                    output.appendLine('\n𓃣 ═══ Workspace Scan Results ═══');
                    output.appendLine(stdout);
                    output.show();

                    // Parse summary for notification
                    try {
                        const result = JSON.parse(stdout);
                        const findings = result.total_findings || result.findings?.length || 0;
                        const size = result.total_size_human || 'unknown';
                        vscode.window.showInformationMessage(
                            `𓃣 Scan complete: ${findings} findings, ${size} reclaimable`
                        );
                    } catch {
                        vscode.window.showInformationMessage('𓃣 Scan complete — see Output panel');
                    }
                } catch (err: unknown) {
                    handleCommandError('Scan', err, output);
                }
            });
        })
    );

    // ── Start Guardian ────────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.guard', async () => {
            const terminal = vscode.window.createTerminal({
                name: '𓁵 Pantheon Guardian',
                shellPath: binaryPath,
                shellArgs: ['guard', '--json'],
                iconPath: new vscode.ThemeIcon('shield'),
            });
            terminal.show();
            output.appendLine('𓁵 Guardian terminal started — watch mode');
        })
    );

    // ── Renice LSP ────────────────────────────────────────────────
    // Uses native renice(1) + taskpolicy(1) — no CLI binary dependency
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.reniceLSP', async () => {
            await vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: '𓁵 Renicing LSP processes...',
                cancellable: false,
            }, async () => {
                if (guardian) {
                    await guardian.manualRenice();
                } else {
                    // Fallback: create a temporary Guardian for one-shot renice
                    const tempGuardian = new Guardian(binaryPath, output, {
                        reniceDelaySec: 0,
                        pollIntervalSec: 5,
                        autoRenice: false,
                    });
                    await tempGuardian.manualRenice();
                    tempGuardian.dispose();
                }
            });
        })
    );

    // ── Ghost Report (Ka) ─────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.ghostReport', async () => {
            await vscode.window.withProgress({
                location: vscode.ProgressLocation.Notification,
                title: '𓂓 Scanning for ghost applications...',
                cancellable: false,
            }, async () => {
                try {
                    const { stdout } = await execFileAsync(binaryPath, [
                        'ghosts', '--json'
                    ], { timeout: 30000 });

                    output.appendLine('\n𓂓 ═══ Ghost Report (Ka) ═══');
                    output.appendLine(stdout);
                    output.show();

                    try {
                        const result = JSON.parse(stdout);
                        const ghosts = result.total_ghosts || result.ghosts?.length || 0;
                        vscode.window.showInformationMessage(
                            `𓂓 Ka found ${ghosts} ghost application(s)`
                        );
                    } catch {
                        vscode.window.showInformationMessage('𓂓 Ghost scan complete — see Output panel');
                    }
                } catch (err: unknown) {
                    handleCommandError('Ka Ghost Report', err, output);
                }
            });
        })
    );

    // ── Thoth Context ─────────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.thothContext', async () => {
            if (!thothProvider || !thothProvider.isLoaded()) {
                vscode.window.showWarningMessage(
                    '𓁟 Thoth: No .thoth/memory.yaml found in workspace'
                );
                return;
            }

            const choice = await vscode.window.showQuickPick([
                { label: '$(book) View Full Memory', description: 'Opens memory.yaml', action: 'full' },
                { label: '$(zap) Copy Compressed Context', description: 'Copies key facts to clipboard', action: 'compressed' },
                { label: '$(info) Show Summary', description: 'Quick summary notification', action: 'summary' },
            ], { placeHolder: '𓁟 Thoth — Context Compression' });

            if (!choice) { return; }

            switch ((choice as { action: string }).action) {
                case 'full':
                    await thothProvider.showContextPanel();
                    break;
                case 'compressed': {
                    const compressed = thothProvider.getCompressedContext();
                    if (compressed) {
                        await vscode.env.clipboard.writeText(compressed);
                        vscode.window.showInformationMessage('𓁟 Compressed context copied to clipboard');
                    }
                    break;
                }
                case 'summary':
                    vscode.window.showInformationMessage(`𓁟 ${thothProvider.getSummary()}`);
                    break;
            }
        })
    );

    // ── Thoth Accountability Report ───────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.thothAccountability', async () => {
            if (!accountabilityEngine) {
                vscode.window.showWarningMessage(
                    '𓁟 Thoth Accountability Engine not initialized'
                );
                return;
            }

            const choice = await vscode.window.showQuickPick([
                { label: '$(graph) Full Accountability Report', description: 'Webview with all metrics, savings, and freshness', action: 'report' },
                { label: '$(pulse) Quick Summary', description: 'Token savings and dollar amount', action: 'summary' },
                { label: '$(clock) Check Freshness', description: 'Is memory.yaml up to date?', action: 'freshness' },
                { label: '$(checklist) Check Coverage', description: 'Are all modules documented?', action: 'coverage' },
                { label: '$(history) Lifetime Stats', description: 'Total savings across all sessions', action: 'lifetime' },
            ], { placeHolder: '𓁟 Thoth Accountability — Choose Report' });

            if (!choice) { return; }

            switch ((choice as { action: string }).action) {
                case 'report':
                    await vscode.window.withProgress({
                        location: vscode.ProgressLocation.Notification,
                        title: '𓁟 Generating Thoth Accountability Report...',
                        cancellable: false,
                    }, async () => {
                        await accountabilityEngine!.showAccountabilityReport();
                    });
                    break;

                case 'summary': {
                    const summary = accountabilityEngine.getSavingsSummary();
                    vscode.window.showInformationMessage(`𓁟 ${summary}`);
                    break;
                }

                case 'freshness': {
                    const freshness = await accountabilityEngine.checkFreshness();
                    if (freshness) {
                        vscode.window.showInformationMessage(
                            `𓁟 Freshness: ${freshness.status} — memory.yaml is ${freshness.ageMinutes} min behind latest source edit (${freshness.latestSourceFile})`
                        );
                    } else {
                        vscode.window.showWarningMessage('𓁟 Unable to check freshness');
                    }
                    break;
                }

                case 'coverage': {
                    const coverage = await accountabilityEngine.checkCoverage();
                    if (coverage) {
                        const missing = coverage.missingModules.length > 0
                            ? ` Missing: ${coverage.missingModules.join(', ')}`
                            : ' All modules covered!';
                        vscode.window.showInformationMessage(
                            `𓁟 Coverage: ${coverage.coveragePercent}% (${coverage.modulesInMemory}/${coverage.modulesOnDisk} modules).${missing}`
                        );
                    } else {
                        vscode.window.showWarningMessage('𓁟 Unable to check coverage');
                    }
                    break;
                }

                case 'lifetime': {
                    const lifetime = accountabilityEngine.getLifetime();
                    vscode.window.showInformationMessage(
                        `𓁟 Lifetime: $${lifetime.totalDollarsSaved.toFixed(2)} saved across ${lifetime.totalSessions} sessions ` +
                        `(${lifetime.totalTokensSaved.toLocaleString()} tokens, since ${new Date(lifetime.firstSessionDate).toLocaleDateString()})`
                    );
                    break;
                }
            }
        })
    );

    // ── System Metrics Dashboard ──────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.showMetrics', async () => {
            if (!statusBar) {
                vscode.window.showWarningMessage('𓃣 Pantheon: Status bar not initialized');
                return;
            }

            const metrics = statusBar.getMetrics();

            // Build items list — include Thoth accountability if available
            const items: vscode.QuickPickItem[] = [
                {
                    label: `$(dashboard) LSP RAM: ${metrics.ramHuman}`,
                    description: `${metrics.lspProcesses} processes`,
                },
                {
                    label: `$(pulse) CPU Hogs: ${metrics.cpuHogs}`,
                    description: 'Processes above 80% CPU',
                },
                {
                    label: `$(shield) Guardian: ${metrics.guardianActive ? '🟢 Active' : '🔴 Stopped'}`,
                    description: 'Always-on process controller',
                },
                {
                    label: `$(clock) Last Update: ${metrics.lastUpdate.toLocaleTimeString()}`,
                    description: '',
                },
            ];

            // Add Thoth savings to the dashboard
            if (accountabilityEngine) {
                const benchmark = accountabilityEngine.getBenchmark();
                if (benchmark) {
                    const summary = accountabilityEngine.getSavingsSummary();
                    items.push({
                        label: `$(bookmark) Thoth: ${summary}`,
                        description: `${benchmark.compressionRatio.toFixed(1)}% compression`,
                    });
                }
            }

            items.push(
                { label: '', kind: vscode.QuickPickItemKind.Separator },
                {
                    label: '$(graph) Thoth Accountability Report',
                    description: 'Full savings + freshness + coverage',
                },
                {
                    label: '$(warning) Crashpad Stability Report',
                    description: 'IDE crash dump monitor',
                },
                {
                    label: '$(arrow-down) Renice LSP Processes',
                    description: 'Lower priority of language servers',
                },
                {
                    label: '$(search) Scan Workspace',
                    description: 'Full infrastructure scan',
                },
                {
                    label: '$(refresh) Refresh Metrics',
                    description: 'Force metric refresh',
                },
            );

            const selected = await vscode.window.showQuickPick(items, {
                placeHolder: '𓃣 Pantheon — System Metrics',
            });

            if (!selected) { return; }

            if (selected.label.includes('Thoth Accountability')) {
                await vscode.commands.executeCommand('sirsi.thothAccountability');
            } else if (selected.label.includes('Crashpad')) {
                await vscode.commands.executeCommand('sirsi.crashpadReport');
            } else if (selected.label.includes('Renice')) {
                await vscode.commands.executeCommand('sirsi.reniceLSP');
            } else if (selected.label.includes('Scan')) {
                await vscode.commands.executeCommand('sirsi.scan');
            } else if (selected.label.includes('Refresh')) {
                statusBar.forceRefresh();
                vscode.window.showInformationMessage('𓃣 Metrics refreshed');
            }
        })
    );

    // ── Apply Workspace Settings ──────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.applyWorkspaceSettings', async () => {
            const confirm = await vscode.window.showWarningMessage(
                '𓃣 Pantheon will apply optimal IDE settings for Go development. This modifies workspace settings.',
                'Apply',
                'Cancel'
            );

            if (confirm !== 'Apply') { return; }

            const wsConfig = vscode.workspace.getConfiguration();

            // gopls directory filters
            await wsConfig.update('gopls.directoryFilters', [
                '-**/node_modules',
                '-**/.git',
                '-**/vendor',
                '-**/.vscode-test',
                '-**/dist',
            ], vscode.ConfigurationTarget.Workspace);

            // File watcher exclusions
            const excludes = wsConfig.get<Record<string, boolean>>('files.watcherExclude') || {};
            excludes['**/node_modules/**'] = true;
            excludes['**/.git/objects/**'] = true;
            excludes['**/.git/subtree-cache/**'] = true;
            excludes['**/dist/**'] = true;
            excludes['**/coverage/**'] = true;
            await wsConfig.update('files.watcherExclude', excludes, vscode.ConfigurationTarget.Workspace);

            // Disable shell integration
            await wsConfig.update(
                'terminal.integrated.shellIntegration.enabled',
                false,
                vscode.ConfigurationTarget.Workspace
            );

            output.appendLine('𓃣 Workspace settings optimized');
            vscode.window.showInformationMessage(
                '𓃣 Workspace settings optimized — gopls filters, watcher exclusions, shell integration'
            );
        })
    );

    // ── Crashpad Stability Report ─────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('sirsi.crashpadReport', async () => {
            if (!crashpadMonitor) {
                vscode.window.showWarningMessage(
                    '𓁵 Crashpad Monitor not initialized'
                );
                return;
            }

            const choice = await vscode.window.showQuickPick([
                { label: '$(warning) Full Stability Report', description: 'Crash dump timeline, trends, and recommendations', action: 'report' },
                { label: '$(pulse) Quick Status', description: 'Pending dump count and trend', action: 'status' },
                { label: '$(trash) Clear Pending Dumps', description: 'Delete unsubmitted crash reports', action: 'clear' },
            ], { placeHolder: '𓁵 Crashpad — IDE Stability Monitor' });

            if (!choice) { return; }

            switch ((choice as { action: string }).action) {
                case 'report':
                    await crashpadMonitor.showReport();
                    break;
                case 'status': {
                    const status = crashpadMonitor.getStatus();
                    if (status) {
                        const trendIcon = status.trend === 'critical' ? '🔴'
                            : status.trend === 'growing' ? '🟡'
                            : '🟢';
                        vscode.window.showInformationMessage(
                            `𓁵 Crashpad: ${status.pendingCount} pending dumps, ` +
                            `${status.recentCount} in last 24h — ${trendIcon} ${status.trend}`
                        );
                    } else {
                        vscode.window.showInformationMessage('𓁵 Crashpad: No data yet — check will run shortly');
                    }
                    break;
                }
                case 'clear':
                    await crashpadMonitor.clearPendingDumps();
                    break;
            }
        })
    );
}

// ── Error Handling ────────────────────────────────────────────────

function handleCommandError(
    command: string,
    err: unknown,
    output: vscode.OutputChannel
): void {
    const msg = err instanceof Error ? err.message : String(err);

    if (msg.includes('ENOENT')) {
        vscode.window.showErrorMessage(
            `𓃣 Pantheon binary not found. Install: brew install sirsi-pantheon`
        );
    } else if (msg.includes('TIMEOUT') || msg.includes('timeout')) {
        vscode.window.showWarningMessage(`𓃣 ${command} timed out`);
    } else {
        vscode.window.showErrorMessage(`𓃣 ${command} failed: ${msg}`);
    }

    output.appendLine(`𓃣 ${command} error: ${msg}`);
}
