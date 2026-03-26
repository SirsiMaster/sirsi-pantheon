// 𓁵 Pantheon Guardian — guardian.ts
//
// Always-on background process controller. The Guardian runs continuously
// in the Extension Host and manages:
//
//   1. Auto-renice: Deprioritizes LSP processes 30s after activation
//   2. Memory pressure monitoring: Watches for sustained RAM hogs
//   3. Re-renice loop: Re-applies renice when processes respawn/reset
//
// Architecture:
//   Guardian.start()
//     └─ setTimeout(reniceDelay) → execRenice()
//     └─ setInterval(pollInterval * 12) → execRenice() (re-apply)
//
// The Guardian never kills processes. It uses renice(1) and taskpolicy(1)
// (macOS) to lower scheduler priority — processes continue running but
// yield CPU to the IDE Renderer on contention.
//
// Safety:
//   - Only renices processes owned by the current user
//   - language_server_macos_arm is PROTECTED from slay but CAN be reniced
//   - No binary modification (Rule A19)
//   - No telemetry (Rule A11)
//   - Uses native renice(1)/taskpolicy(1) — no CLI binary dependency

import { execFile } from 'child_process';
import { promisify } from 'util';
import * as vscode from 'vscode';

const execFileAsync = promisify(execFile);

export interface GuardianConfig {
    reniceDelaySec: number;
    pollIntervalSec: number;
    autoRenice: boolean;
}

// GC thresholds for memory pressure response
const GC_THRESHOLD_BYTES = 500 * 1024 * 1024;   // 500 MB per third-party LSP
const GC_SUSTAINED_CHECKS = 3;                    // Must exceed threshold for 3 consecutive checks
const HOST_LSP = 'language_server_macos_arm';      // Excluded from GC (Antigravity core)

export interface ReniceResult {
    target: string;
    reniced: number;
    skipped: number;
    errors?: string[];
    processes?: ReniceProcess[];
}

export interface ReniceProcess {
    pid: number;
    name: string;
    rss_bytes: number;
    rss_human: string;
    old_nice: number;
    new_nice: number;
    qos: string;
}

export interface GuardMetrics {
    totalRAM: number;
    totalRAMHuman: string;
    processCount: number;
    reniceCount: number;
    lastRenice: Date | null;
    guardianUptime: number;
    errors: string[];
}

// LSP process patterns to target for deprioritization
const LSP_PATTERNS = [
    'gopls',
    'typescript-language-server',
    'tsserver',
    'pylsp',
    'rust-analyzer',
    'clangd',
    'sourcekit-lsp',
    'language_server_macos_arm',
    'dart-language-server',
    'vscode-json-language',
    'vscode-css-language',
    'vscode-html-language',
    'eslint',
];

export class Guardian implements vscode.Disposable {
    private binaryPath: string;
    private output: vscode.OutputChannel;
    private config: GuardianConfig;
    private reniceTimer: NodeJS.Timeout | undefined;
    private pollTimer: NodeJS.Timeout | undefined;
    private disposed = false;
    private startTime: Date = new Date();
    private lastReniceTime: Date | null = null;
    private totalReniced = 0;
    private lastErrors: string[] = [];
    private pressureMap: Map<string, number> = new Map(); // processName → consecutive over-threshold count
    private gcCount = 0;

    constructor(
        binaryPath: string,
        output: vscode.OutputChannel,
        config: GuardianConfig
    ) {
        this.binaryPath = binaryPath;
        this.output = output;
        this.config = config;
    }

    // ── Lifecycle ─────────────────────────────────────────────────

    start(): void {
        this.startTime = new Date();
        this.output.appendLine(`𓁵 Guardian starting — delay ${this.config.reniceDelaySec}s`);

        // Schedule initial renice after delay (LSPs need time to spawn)
        this.reniceTimer = setTimeout(async () => {
            if (this.disposed) { return; }
            await this.executeRenice();

            // Start recurring poll loop (renice + GC check)
            if (this.config.autoRenice && !this.disposed) {
                this.pollTimer = setInterval(async () => {
                    if (this.disposed) { return; }
                    await this.executeRenice();
                    await this.checkMemoryPressure();
                }, this.config.pollIntervalSec * 1000 * 12); // Re-renice every 12 poll intervals (60s default)
            }
        }, this.config.reniceDelaySec * 1000);
    }

    dispose(): void {
        this.disposed = true;
        if (this.reniceTimer) {
            clearTimeout(this.reniceTimer);
            this.reniceTimer = undefined;
        }
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = undefined;
        }
        this.output.appendLine('𓁵 Guardian stopped');
    }

    // ── Renice Execution ──────────────────────────────────────────
    // Uses native macOS renice(1) and taskpolicy(1) directly.
    // Does NOT depend on the Pantheon CLI binary for renice.

    async executeRenice(): Promise<ReniceResult | null> {
        try {
            // Step 1: Discover LSP processes using `ps`
            const { stdout } = await execFileAsync('ps', [
                '-axo', 'pid,ni,rss,%cpu,comm'
            ], { timeout: 5000 });

            const lines = stdout.split('\n');
            const lspProcesses: Array<{
                pid: number;
                nice: number;
                rss: number;
                cpu: number;
                name: string;
            }> = [];

            for (let i = 1; i < lines.length; i++) {
                const line = lines[i].trim();
                if (!line) { continue; }

                const fields = line.split(/\s+/);
                if (fields.length < 5) { continue; }

                const pid = parseInt(fields[0], 10);
                const nice = parseInt(fields[1], 10);
                const rss = parseInt(fields[2], 10) * 1024; // KB → bytes
                const name = fields.slice(4).join(' ').toLowerCase();

                const isLSP = LSP_PATTERNS.some(p => name.includes(p));
                if (isLSP) {
                    lspProcesses.push({ pid, nice, rss, cpu: parseFloat(fields[3]), name });
                }
            }

            if (lspProcesses.length === 0) {
                this.output.appendLine('𓁵 Guardian: No LSP processes found');
                return {
                    target: 'lsp',
                    reniced: 0,
                    skipped: 0,
                    processes: [],
                };
            }

            // Step 2: Renice each LSP process to nice +10
            const reniced: ReniceProcess[] = [];
            const errors: string[] = [];
            let skipped = 0;

            for (const proc of lspProcesses) {
                if (proc.nice >= 10) {
                    // Already deprioritized
                    skipped++;
                    continue;
                }

                try {
                    // Apply nice level +10
                    await execFileAsync('renice', [
                        '-n', '10', '-p', String(proc.pid)
                    ], { timeout: 3000 });

                    // Apply Background QoS via taskpolicy (macOS only)
                    try {
                        await execFileAsync('taskpolicy', [
                            '-b', '-p', String(proc.pid)
                        ], { timeout: 3000 });
                    } catch {
                        // taskpolicy may not be available on all macOS versions
                    }

                    reniced.push({
                        pid: proc.pid,
                        name: this.extractBaseName(proc.name),
                        rss_bytes: proc.rss,
                        rss_human: this.formatBytes(proc.rss),
                        old_nice: proc.nice,
                        new_nice: 10,
                        qos: 'background',
                    });
                } catch (err: unknown) {
                    const msg = err instanceof Error ? err.message : String(err);
                    errors.push(`PID ${proc.pid} (${this.extractBaseName(proc.name)}): ${msg}`);
                }
            }

            const result: ReniceResult = {
                target: 'lsp',
                reniced: reniced.length,
                skipped,
                errors: errors.length > 0 ? errors : undefined,
                processes: reniced,
            };

            this.lastReniceTime = new Date();
            this.totalReniced += result.reniced;

            if (result.reniced > 0) {
                const totalRAM = reniced.reduce((sum, p) => sum + p.rss_bytes, 0);
                const ramHuman = this.formatBytes(totalRAM);
                const names = reniced.map(p => `${p.name} (${p.rss_human})`).join(', ');
                this.output.appendLine(
                    `𓁵 Guardian reniced ${result.reniced} process(es): ${names}`
                );
                this.output.appendLine(`   Total RAM of reniced processes: ${ramHuman}`);
            } else if (skipped > 0) {
                this.output.appendLine(
                    `𓁵 Guardian: ${skipped} process(es) already at nice ≥10`
                );
            }

            this.lastErrors = errors;
            return result;
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            this.output.appendLine(`𓁵 Guardian renice error: ${msg}`);
            this.lastErrors = [msg];
            return null;
        }
    }

    // ── Manual trigger (from command palette) ─────────────────────

    async manualRenice(): Promise<void> {
        const result = await this.executeRenice();
        if (result) {
            if (result.reniced === 0 && result.skipped === 0) {
                vscode.window.showInformationMessage(
                    '𓁵 No LSP processes found to renice'
                );
            } else if (result.reniced === 0 && result.skipped > 0) {
                vscode.window.showInformationMessage(
                    `𓁵 ${result.skipped} LSP process(es) already deprioritized`
                );
            } else {
                const procs = result.processes
                    ?.map(p => `${p.name} (${p.rss_human})`)
                    .join(', ') || '';
                vscode.window.showInformationMessage(
                    `𓁵 Reniced ${result.reniced} process(es): ${procs}`
                );
            }
        } else {
            vscode.window.showWarningMessage(
                '𓁵 Guardian: Renice failed — check Output panel for details'
            );
        }
    }

    // ── Memory Pressure GC ────────────────────────────────────────
    // Tracks RSS per-process across poll cycles. When a third-party LSP
    // stays above GC_THRESHOLD_BYTES for GC_SUSTAINED_CHECKS consecutive
    // polls, triggers VS Code's built-in LSP restart to reclaim memory.
    // The host LSP (language_server_macos_arm) is EXCLUDED from GC.

    private async checkMemoryPressure(): Promise<void> {
        try {
            const { stdout } = await execFileAsync('ps', [
                '-axo', 'pid,rss,comm'
            ], { timeout: 5000 });

            const lines = stdout.split('\n');
            const currentBloated = new Set<string>();

            for (let i = 1; i < lines.length; i++) {
                const line = lines[i].trim();
                if (!line) { continue; }

                const fields = line.split(/\s+/);
                if (fields.length < 3) { continue; }

                const rss = parseInt(fields[1], 10) * 1024; // KB → bytes
                const name = fields.slice(2).join(' ').toLowerCase();
                const baseName = this.extractBaseName(name);

                // Skip host LSP — Antigravity core, not eligible for GC
                if (name.includes(HOST_LSP)) { continue; }

                // Only check known LSP patterns
                const isLSP = LSP_PATTERNS.some(p => name.includes(p));
                if (!isLSP) { continue; }

                if (rss > GC_THRESHOLD_BYTES) {
                    currentBloated.add(baseName);
                    const count = (this.pressureMap.get(baseName) || 0) + 1;
                    this.pressureMap.set(baseName, count);

                    if (count >= GC_SUSTAINED_CHECKS) {
                        // Sustained bloat — trigger GC
                        this.output.appendLine(
                            `𓁵 GC: ${baseName} sustained ${this.formatBytes(rss)} for ${count} checks — triggering restart`
                        );
                        await this.triggerLSPRestart(baseName);
                        this.pressureMap.delete(baseName);
                        this.gcCount++;
                    }
                }
            }

            // Reset pressure count for processes that dropped below threshold
            for (const [name] of this.pressureMap) {
                if (!currentBloated.has(name)) {
                    this.pressureMap.delete(name);
                }
            }
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            this.output.appendLine(`𓁵 GC check error: ${msg}`);
        }
    }

    private async triggerLSPRestart(processName: string): Promise<void> {
        // Map LSP process names to VS Code restart commands
        const restartCommands: Record<string, string> = {
            'gopls': 'go.languageserver.restart',
            'typescript-language-server': 'typescript.restartTsServer',
            'tsserver': 'typescript.restartTsServer',
            'eslint': 'eslint.restart',
            'rust-analyzer': 'rust-analyzer.reload',
            'clangd': 'clangd.restart',
            'pylsp': 'python.analysis.restartLanguageServer',
            'sourcekit-lsp': 'swift.restartLanguageServer',
        };

        const command = restartCommands[processName];
        if (command) {
            try {
                await vscode.commands.executeCommand(command);
                this.output.appendLine(`𓁵 GC: Restarted ${processName} via ${command}`);
                vscode.window.showInformationMessage(
                    `𓁵 Guardian GC: Restarted ${processName} to reclaim memory`
                );
            } catch {
                this.output.appendLine(`𓁵 GC: Command ${command} not available — ${processName} may not be installed`);
            }
        } else {
            this.output.appendLine(`𓁵 GC: No restart command mapped for ${processName}`);
        }
    }

    // ── Metrics ───────────────────────────────────────────────────

    getMetrics(): GuardMetrics {
        return {
            totalRAM: 0,
            totalRAMHuman: '—',
            processCount: 0,
            reniceCount: this.totalReniced,
            lastRenice: this.lastReniceTime,
            guardianUptime: Date.now() - this.startTime.getTime(),
            errors: this.lastErrors,
        };
    }

    // ── Utilities ─────────────────────────────────────────────────

    private extractBaseName(fullPath: string): string {
        const parts = fullPath.split('/');
        return parts[parts.length - 1] || fullPath;
    }

    private formatBytes(bytes: number): string {
        if (bytes === 0) { return '0 B'; }
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const k = 1024;
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return `${(bytes / Math.pow(k, i)).toFixed(1)} ${units[i]}`;
    }
}
