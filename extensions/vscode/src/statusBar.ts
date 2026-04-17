// 𓃣 Pantheon Status Bar — statusBar.ts
//
// The Ankh (𓃣) icon in the IDE status bar with live system metrics.
//
// States:
//   𓃣 Pantheon      — healthy, Guardian running
//   𓃣 Pantheon ▲    — memory pressure detected
//   ⚠ Pantheon      — error / binary not found
//   ⏳ Pantheon      — initializing / first scan pending
//
// Metrics shown on hover tooltip:
//   - Total RAM used by LSP processes
//   - Guardian uptime
//   - Last renice timestamp
//   - Active deity count
//
// Click → opens quick picker with Guardian options

import { execFile } from 'child_process';
import { promisify } from 'util';
import * as vscode from 'vscode';

const execFileAsync = promisify(execFile);

interface SystemMetrics {
    totalRAM: number;
    ramHuman: string;
    lspProcesses: number;
    cpuHogs: number;
    guardianActive: boolean;
    lastUpdate: Date;
}

type StatusState = 'healthy' | 'warning' | 'error' | 'initializing';

export class PantheonStatusBar implements vscode.Disposable {
    private statusBarItem: vscode.StatusBarItem;
    private binaryPath: string;
    private output: vscode.OutputChannel;
    private pollTimer: NodeJS.Timeout | undefined;
    private state: StatusState = 'initializing';
    private metrics: SystemMetrics = {
        totalRAM: 0,
        ramHuman: '—',
        lspProcesses: 0,
        cpuHogs: 0,
        guardianActive: false,
        lastUpdate: new Date(),
    };

    constructor(binaryPath: string, output: vscode.OutputChannel) {
        this.binaryPath = binaryPath;
        this.output = output;

        this.statusBarItem = vscode.window.createStatusBarItem(
            vscode.StatusBarAlignment.Right,
            200 // High priority — always visible
        );
        this.statusBarItem.command = 'sirsi.showMetrics';
        this.updateDisplay();
        this.statusBarItem.show();
    }

    // ── Metric Refresh Loop ───────────────────────────────────────

    startMetricLoop(intervalMs: number): void {
        // Immediate first fetch
        this.fetchMetrics();

        this.pollTimer = setInterval(() => {
            this.fetchMetrics();
        }, intervalMs);
    }

    private async fetchMetrics(): Promise<void> {
        try {
            // Use `ps` directly for lightweight metric collection
            // (avoid spawning full Pantheon binary every 5s)
            const { stdout } = await execFileAsync('ps', [
                '-axo', 'pid,rss,%cpu,comm'
            ], { timeout: 5000, maxBuffer: 1024 * 1024 }); // 1MB buffer for large process lists

            const lines = stdout.split('\n');
            let totalRAM = 0;
            let lspCount = 0;
            let cpuHogs = 0;
            let thirdPartyRAM = 0; // RAM from non-host LSPs only

            const lspPatterns = [
                'language_server_macos_arm',
                'gopls',
                'typescript-language-server',
                'pylsp',
                'rust-analyzer',
                'clangd',
                'sourcekit-lsp',
            ];

            // Host IDE LSP — always large, not a warning signal
            const hostLSP = 'language_server_macos_arm';

            for (let i = 1; i < lines.length; i++) {
                const line = lines[i].trim();
                if (!line) { continue; }

                const fields = line.split(/\s+/);
                if (fields.length < 4) { continue; }

                const rss = parseInt(fields[1], 10) * 1024; // KB → bytes
                const cpu = parseFloat(fields[2]);
                const name = fields.slice(3).join(' ').toLowerCase();

                const isLSP = lspPatterns.some(p => name.includes(p));
                if (isLSP) {
                    totalRAM += rss;
                    lspCount++;
                    if (!name.includes(hostLSP)) {
                        thirdPartyRAM += rss;
                    }
                }

                if (cpu > 80) {
                    cpuHogs++;
                }
            }

            this.metrics = {
                totalRAM,
                ramHuman: this.formatBytes(totalRAM),
                lspProcesses: lspCount,
                cpuHogs,
                guardianActive: true,
                lastUpdate: new Date(),
            };

            // Warning based on third-party LSPs only (host LSP at 4-6 GB is normal)
            if (thirdPartyRAM > 1 * 1024 * 1024 * 1024) { // > 1 GB of third-party LSPs
                this.state = 'warning';
            } else {
                this.state = 'healthy';
            }

            this.updateDisplay();
        } catch {
            this.state = 'error';
            this.updateDisplay();
        }
    }

    // ── Display ───────────────────────────────────────────────────

    private updateDisplay(): void {
        switch (this.state) {
            case 'healthy':
                this.statusBarItem.text = `$(eye) PANTHEON ${this.metrics.ramHuman}`;
                this.statusBarItem.backgroundColor = undefined;
                break;
            case 'warning':
                this.statusBarItem.text = `$(eye) PANTHEON ${this.metrics.ramHuman} $(warning)`;
                this.statusBarItem.backgroundColor = new vscode.ThemeColor(
                    'statusBarItem.warningBackground'
                );
                break;
            case 'error':
                this.statusBarItem.text = `$(warning) PANTHEON ${this.metrics.ramHuman}`;
                this.statusBarItem.backgroundColor = new vscode.ThemeColor(
                    'statusBarItem.errorBackground'
                );
                break;
            case 'initializing':
                this.statusBarItem.text = '$(loading~spin) PANTHEON';
                this.statusBarItem.backgroundColor = undefined;
                break;
        }

        this.statusBarItem.tooltip = this.buildTooltip();
    }

    private buildTooltip(): vscode.MarkdownString {
        const md = new vscode.MarkdownString();
        md.isTrusted = true;
        md.supportThemeIcons = true;

        md.appendMarkdown('### 𓃣 Pantheon — Anubis Suite\n\n');
        md.appendMarkdown(`| Metric | Value |\n|--------|-------|\n`);
        md.appendMarkdown(`| LSP RAM | **${this.metrics.ramHuman}** |\n`);
        md.appendMarkdown(`| LSP Processes | ${this.metrics.lspProcesses} |\n`);
        md.appendMarkdown(`| CPU Hogs (>80%) | ${this.metrics.cpuHogs} |\n`);
        md.appendMarkdown(`| Guardian | ${this.metrics.guardianActive ? '🟢 Active' : '🔴 Stopped'} |\n`);
        md.appendMarkdown(`| Last Update | ${this.metrics.lastUpdate.toLocaleTimeString()} |\n`);
        md.appendMarkdown('\n---\n');
        md.appendMarkdown('$(zap) Click for more options');

        return md;
    }

    // ── Public API ────────────────────────────────────────────────

    getMetrics(): SystemMetrics {
        return { ...this.metrics };
    }

    getState(): StatusState {
        return this.state;
    }

    forceRefresh(): void {
        this.fetchMetrics();
    }

    // ── Cleanup ───────────────────────────────────────────────────

    dispose(): void {
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = undefined;
        }
        this.statusBarItem.dispose();
    }

    // ── Utilities ─────────────────────────────────────────────────

    private formatBytes(bytes: number): string {
        if (bytes === 0) { return '0 B'; }
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const k = 1024;
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return `${(bytes / Math.pow(k, i)).toFixed(1)} ${units[i]}`;
    }
}
