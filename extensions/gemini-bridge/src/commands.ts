// 𓁆 Seshat — Command Registration
//
// Registers all VS Code Command Palette entries for the Gemini Bridge.
// Commands delegate to the Pantheon CLI `seshat` subcommand or
// directly invoke the Python scripts via the skill wrapper.

import * as vscode from 'vscode';
import * as cp from 'child_process';
import * as path from 'path';
import { SeshatPaths } from './paths';
import { KnowledgeItemsProvider } from './knowledgeProvider';
import { ChromeProfilesProvider } from './chromeProfilesProvider';
import { SyncStatusProvider } from './syncStatusProvider';
import { SeshatDashboard } from './dashboard';

export function registerCommands(
    context: vscode.ExtensionContext,
    paths: SeshatPaths,
    outputChannel: vscode.OutputChannel,
    knowledgeProvider: KnowledgeItemsProvider,
    profilesProvider: ChromeProfilesProvider,
    syncProvider: SyncStatusProvider,
    dashboard: SeshatDashboard
): void {
    // ── seshat.listKnowledge ──────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.listKnowledge', async () => {
            try {
                const names = knowledgeProvider.getKINames();
                if (names.length === 0) {
                    vscode.window.showInformationMessage('𓁆 No Knowledge Items found.');
                    return;
                }

                const selected = await vscode.window.showQuickPick(names, {
                    placeHolder: 'Select a Knowledge Item to view',
                    title: '𓁆 Knowledge Items',
                });

                if (selected) {
                    const metaPath = path.join(paths.knowledgeDir, selected, 'metadata.json');
                    const uri = vscode.Uri.file(metaPath);
                    await vscode.commands.executeCommand('vscode.open', uri);
                }
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                outputChannel.appendLine(`𓁆 listKnowledge error: ${msg}`);
                vscode.window.showErrorMessage(`𓁆 Error: ${msg}`);
            }
        })
    );

    // ── seshat.exportKI ───────────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.exportKI', async () => {
            try {
                const names = knowledgeProvider.getKINames();
                if (names.length === 0) {
                    vscode.window.showInformationMessage('𓁆 No Knowledge Items to export.');
                    return;
                }

                const items = ['Export All', ...names];
                const selected = await vscode.window.showQuickPick(items, {
                    placeHolder: 'Select a Knowledge Item to export as Markdown',
                    title: '𓁆 Export Knowledge Item',
                });

                if (!selected) { return; }

                const config = vscode.workspace.getConfiguration('seshat');
                const binaryPath = config.get<string>('sirsiBinaryPath', 'sirsi');

                if (selected === 'Export All') {
                    await runPantheonCommand(binaryPath, ['seshat', 'export', '--all'], outputChannel);
                } else {
                    await runPantheonCommand(binaryPath, ['seshat', 'export', '--ki', selected], outputChannel);
                }

                vscode.window.showInformationMessage(`𓁆 Exported: ${selected}`);
                syncProvider.refresh();
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                outputChannel.appendLine(`𓁆 exportKI error: ${msg}`);
                vscode.window.showErrorMessage(`𓁆 Export error: ${msg}`);
            }
        })
    );

    // ── seshat.syncToGemini ───────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.syncToGemini', async () => {
            try {
                const workspaceFolders = vscode.workspace.workspaceFolders;
                if (!workspaceFolders || workspaceFolders.length === 0) {
                    vscode.window.showWarningMessage('𓁆 No workspace folder open. Open a project first.');
                    return;
                }

                const targetFile = path.join(workspaceFolders[0].uri.fsPath, 'GEMINI.md');

                const names = knowledgeProvider.getKINames();
                if (names.length === 0) {
                    vscode.window.showInformationMessage('𓁆 No Knowledge Items to sync.');
                    return;
                }

                const items = ['Sync All Relevant', ...names];
                const selected = await vscode.window.showQuickPick(items, {
                    placeHolder: 'Select KI to sync to GEMINI.md',
                    title: '𓁆 Sync → GEMINI.md',
                });

                if (!selected) { return; }

                const config = vscode.workspace.getConfiguration('seshat');
                const binaryPath = config.get<string>('sirsiBinaryPath', 'sirsi');

                if (selected === 'Sync All Relevant') {
                    await runPantheonCommand(binaryPath, ['seshat', 'sync', '--target', targetFile], outputChannel);
                } else {
                    await runPantheonCommand(binaryPath, ['seshat', 'sync', '--ki', selected, '--target', targetFile], outputChannel);
                }

                vscode.window.showInformationMessage(`𓁆 Synced to ${path.basename(targetFile)}`);
                syncProvider.refresh();
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                outputChannel.appendLine(`𓁆 syncToGemini error: ${msg}`);
                vscode.window.showErrorMessage(`𓁆 Sync error: ${msg}`);
            }
        })
    );

    // ── seshat.listProfiles ────────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.listProfiles', async () => {
            try {
                const names = profilesProvider.getProfileNames();
                if (names.length === 0) {
                    vscode.window.showInformationMessage('𓁆 No Chrome profiles detected.');
                    return;
                }

                const config = vscode.workspace.getConfiguration('seshat');
                const currentDefault = config.get<string>('defaultProfile', 'SirsiMaster');

                const selected = await vscode.window.showQuickPick(
                    names.map(n => ({
                        label: n === currentDefault ? `★ ${n}` : n,
                        description: n === currentDefault ? 'Current default' : '',
                        profileName: n,
                    })),
                    {
                        placeHolder: 'Select a Chrome profile to set as default',
                        title: '𓁆 Chrome Profiles',
                    }
                );

                if (selected) {
                    await config.update('defaultProfile', selected.profileName, vscode.ConfigurationTarget.Global);
                    vscode.window.showInformationMessage(`𓁆 Default profile set to: ${selected.profileName}`);
                    profilesProvider.refresh();
                }
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                outputChannel.appendLine(`𓁆 listProfiles error: ${msg}`);
                vscode.window.showErrorMessage(`𓁆 Error: ${msg}`);
            }
        })
    );

    // ── seshat.listConversations ───────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.listConversations', async () => {
            try {
                const config = vscode.workspace.getConfiguration('seshat');
                const binaryPath = config.get<string>('sirsiBinaryPath', 'sirsi');

                const output = await runPantheonCommand(
                    binaryPath,
                    ['seshat', 'conversations', '--last', '20'],
                    outputChannel
                );

                // Show in new untitled document
                const doc = await vscode.workspace.openTextDocument({
                    content: `𓁆 Seshat — Recent Brain Conversations\n${'═'.repeat(50)}\n\n${output}`,
                    language: 'markdown',
                });
                await vscode.window.showTextDocument(doc);
            } catch (err) {
                const msg = err instanceof Error ? err.message : String(err);
                outputChannel.appendLine(`𓁆 listConversations error: ${msg}`);
                vscode.window.showErrorMessage(`𓁆 Error: ${msg}`);
            }
        })
    );

    // ── seshat.showDashboard ──────────────────────────────────────────
    context.subscriptions.push(
        vscode.commands.registerCommand('seshat.showDashboard', () => {
            dashboard.show();
        })
    );
}

/**
 * Runs a Pantheon CLI command and returns stdout.
 */
function runPantheonCommand(
    binaryPath: string,
    args: string[],
    outputChannel: vscode.OutputChannel
): Promise<string> {
    return new Promise((resolve, reject) => {
        outputChannel.appendLine(`𓁆 Running: ${binaryPath} ${args.join(' ')}`);

        const proc = cp.spawn(binaryPath, args, {
            env: { ...process.env },
            timeout: 30000,
        });

        let stdout = '';
        let stderr = '';

        proc.stdout.on('data', (data: Buffer) => {
            stdout += data.toString();
        });

        proc.stderr.on('data', (data: Buffer) => {
            stderr += data.toString();
        });

        proc.on('close', (code: number | null) => {
            if (code === 0) {
                outputChannel.appendLine(`𓁆 Command succeeded`);
                resolve(stdout.trim());
            } else {
                const msg = stderr.trim() || `Exit code: ${code}`;
                outputChannel.appendLine(`𓁆 Command failed: ${msg}`);
                reject(new Error(msg));
            }
        });

        proc.on('error', (err: Error) => {
            outputChannel.appendLine(`𓁆 Spawn error: ${err.message}`);
            reject(err);
        });
    });
}
