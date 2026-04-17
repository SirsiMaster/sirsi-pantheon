// 𓁵 Pantheon Crashpad Monitor — crashpadMonitor.ts
//
// Monitors the IDE's Crashpad directory for pending crash dumps.
// A growing pending dump count is a leading indicator of Extension Host
// instability — the kind of silent failure that eventually requires a
// full IDE reinstall.
//
// This module exists because of Case Study 011: Session 22's manifest
// patches caused an Extension Host V8 OOM → Jetsam cascade that generated
// 34 pending crash dumps and required 2 reinstalls. The user had no
// warning — this monitor provides one.
//
// Architecture:
//   CrashpadMonitor.start()
//     └─ Reads Crashpad/pending/ on a 5-minute interval
//     └─ Tracks dump count over time to detect growth trends
//     └─ Warns the user when dumps accumulate (stale or growing)
//     └─ Status bar shows crash stability indicator
//
// Safety:
//   - READ-ONLY — never deletes or modifies crash dumps
//   - No telemetry (Rule A11)
//   - No bundle modification (Rule A19)

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

export interface CrashpadStatus {
    pendingCount: number;
    recentCount: number;       // Dumps from the last 24 hours
    oldestDump: Date | null;
    newestDump: Date | null;
    trend: 'stable' | 'growing' | 'critical';
    crashpadPath: string;
    lastCheck: Date;
    lastExtHostCrash: Date | null;
}

const POLL_INTERVAL_MS = 5 * 60 * 1000;   // 5 minutes
const RECENT_WINDOW_MS = 24 * 60 * 60 * 1000; // 24 hours
const WARNING_THRESHOLD = 5;               // Warn at 5+ pending dumps
const CRITICAL_THRESHOLD = 15;             // Critical at 15+ pending dumps
const GROWTH_WINDOW = 3;                   // Track last 3 readings for trend

export class CrashpadMonitor implements vscode.Disposable {
    private output: vscode.OutputChannel;
    private pollTimer: NodeJS.Timeout | undefined;
    private disposed = false;
    private crashpadPath: string;
    private history: number[] = [];       // Last N readings for trend detection
    private lastStatus: CrashpadStatus | null = null;
    private statusBarItem: vscode.StatusBarItem;
    private hasWarnedThisSession = false;

    constructor(output: vscode.OutputChannel) {
        this.output = output;
        this.crashpadPath = this.detectCrashpadPath();

        // Create a dedicated status bar item for crash stability
        this.statusBarItem = vscode.window.createStatusBarItem(
            vscode.StatusBarAlignment.Right,
            95  // Just after the main Pantheon status bar (priority 100)
        );
        this.statusBarItem.command = 'sirsi.crashpadReport';
    }

    // ── Lifecycle ─────────────────────────────────────────────────

    start(): void {
        if (!this.crashpadPath || !fs.existsSync(this.crashpadPath)) {
            this.output.appendLine('𓁵 Crashpad Monitor: No Crashpad directory found — monitor inactive');
            return;
        }

        this.output.appendLine(`𓁵 Crashpad Monitor armed — watching ${this.crashpadPath}`);

        // Initial check
        this.checkCrashpad().catch(err => {
            const msg = err instanceof Error ? err.message : String(err);
            this.output.appendLine(`𓁵 Crashpad Monitor error: ${msg}`);
        });

        // Recurring poll
        this.pollTimer = setInterval(() => {
            if (this.disposed) { return; }
            this.checkCrashpad().catch(err => {
                const msg = err instanceof Error ? err.message : String(err);
                this.output.appendLine(`𓁵 Crashpad Monitor error: ${msg}`);
            });
        }, POLL_INTERVAL_MS);
    }

    dispose(): void {
        this.disposed = true;
        if (this.pollTimer) {
            clearInterval(this.pollTimer);
            this.pollTimer = undefined;
        }
        this.statusBarItem.dispose();
    }

    // ── Core Check ────────────────────────────────────────────────

    async checkCrashpad(): Promise<CrashpadStatus> {
        const pendingDir = path.join(this.crashpadPath, 'pending');

        if (!fs.existsSync(pendingDir)) {
            const status: CrashpadStatus = {
                pendingCount: 0,
                recentCount: 0,
                oldestDump: null,
                newestDump: null,
                trend: 'stable',
                crashpadPath: this.crashpadPath,
                lastCheck: new Date(),
                lastExtHostCrash: null,
            };
            this.lastStatus = status;
            this.updateStatusBar(status);
            return status;
        }

        // Read all .dmp files
        const entries = fs.readdirSync(pendingDir)
            .filter(f => f.endsWith('.dmp'));

        const now = Date.now();
        let oldestTime: number | null = null;
        let newestTime: number | null = null;
        let recentCount = 0;
        let lastExtHostCrash: Date | null = null;

        for (const entry of entries) {
            const filePath = path.join(pendingDir, entry);
            try {
                const stats = fs.statSync(filePath);
                const mtime = stats.mtimeMs;

                if (oldestTime === null || mtime < oldestTime) {
                    oldestTime = mtime;
                }
                if (newestTime === null || mtime > newestTime) {
                    newestTime = mtime;
                }
                if (now - mtime < RECENT_WINDOW_MS) {
                    recentCount++;
                }

                // Quick check: is this an Extension Host crash?
                // Read first 8KB for VSCODE_CRASH_REPORTER_PROCESS_TYPE
                if (now - mtime < RECENT_WINDOW_MS) {
                    try {
                        const fd = fs.openSync(filePath, 'r');
                        const buffer = Buffer.alloc(8192);
                        fs.readSync(fd, buffer, 0, 8192, 0);
                        fs.closeSync(fd);
                        const snippet = buffer.toString('ascii', 0, 8192);
                        if (snippet.includes('extensionHost')) {
                            if (!lastExtHostCrash || mtime > lastExtHostCrash.getTime()) {
                                lastExtHostCrash = new Date(mtime);
                            }
                        }
                    } catch {
                        // Ignore read errors on individual dumps
                    }
                }
            } catch {
                // Ignore stat errors
            }
        }

        // Trend detection
        this.history.push(entries.length);
        if (this.history.length > GROWTH_WINDOW) {
            this.history.shift();
        }

        let trend: 'stable' | 'growing' | 'critical' = 'stable';
        if (entries.length >= CRITICAL_THRESHOLD) {
            trend = 'critical';
        } else if (entries.length >= WARNING_THRESHOLD) {
            trend = 'growing';
        } else if (this.history.length >= 2) {
            const prev = this.history[this.history.length - 2];
            const curr = this.history[this.history.length - 1];
            if (curr > prev && curr >= 3) {
                trend = 'growing';
            }
        }

        const status: CrashpadStatus = {
            pendingCount: entries.length,
            recentCount,
            oldestDump: oldestTime ? new Date(oldestTime) : null,
            newestDump: newestTime ? new Date(newestTime) : null,
            trend,
            crashpadPath: this.crashpadPath,
            lastCheck: new Date(),
            lastExtHostCrash,
        };

        this.lastStatus = status;
        this.updateStatusBar(status);

        // Log the check
        if (status.pendingCount > 0) {
            this.output.appendLine(
                `𓁵 Crashpad: ${status.pendingCount} pending dumps ` +
                `(${status.recentCount} in last 24h) — trend: ${status.trend}`
            );
        }

        // Warn the user if things are degrading (once per session)
        if (!this.hasWarnedThisSession && status.trend !== 'stable') {
            this.hasWarnedThisSession = true;

            if (status.trend === 'critical') {
                const action = await vscode.window.showWarningMessage(
                    `𓁵 Crashpad Alert: ${status.pendingCount} pending crash dumps detected. ` +
                    `Your IDE may be chronically unstable. ${status.recentCount} crashes in the last 24 hours.`,
                    'View Report',
                    'Clear Pending',
                    'Dismiss'
                );
                if (action === 'View Report') {
                    vscode.commands.executeCommand('sirsi.crashpadReport');
                } else if (action === 'Clear Pending') {
                    await this.clearPendingDumps();
                }
            } else if (status.trend === 'growing') {
                const action = await vscode.window.showInformationMessage(
                    `𓁵 Crashpad: ${status.pendingCount} pending dumps accumulating. ` +
                    `This may indicate Extension Host instability.`,
                    'View Report',
                    'Dismiss'
                );
                if (action === 'View Report') {
                    vscode.commands.executeCommand('sirsi.crashpadReport');
                }
            }
        }

        return status;
    }

    // ── Status Bar ────────────────────────────────────────────────

    private updateStatusBar(status: CrashpadStatus): void {
        if (status.trend === 'stable' && status.pendingCount < 3) {
            // Hide when everything is fine — don't clutter
            this.statusBarItem.hide();
            return;
        }

        this.statusBarItem.show();

        if (status.trend === 'critical') {
            this.statusBarItem.text = `$(warning) ${status.pendingCount} crashes`;
            this.statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.errorBackground');
            this.statusBarItem.tooltip = `Crashpad: ${status.pendingCount} pending dumps — IDE stability at risk`;
        } else if (status.trend === 'growing') {
            this.statusBarItem.text = `$(alert) ${status.pendingCount} dumps`;
            this.statusBarItem.backgroundColor = new vscode.ThemeColor('statusBarItem.warningBackground');
            this.statusBarItem.tooltip = `Crashpad: ${status.pendingCount} pending dumps accumulating`;
        } else {
            this.statusBarItem.text = `$(info) ${status.pendingCount} dumps`;
            this.statusBarItem.backgroundColor = undefined;
            this.statusBarItem.tooltip = `Crashpad: ${status.pendingCount} stale pending dumps`;
        }
    }

    // ── Webview Report ────────────────────────────────────────────

    async showReport(): Promise<void> {
        // Force a fresh check
        const status = await this.checkCrashpad();

        const panel = vscode.window.createWebviewPanel(
            'pantheonCrashpad',
            '𓁵 Crashpad Stability Report',
            vscode.ViewColumn.One,
            { enableScripts: false }
        );

        panel.webview.html = this.generateReportHTML(status);
    }

    private generateReportHTML(status: CrashpadStatus): string {
        const trendIcon = status.trend === 'critical' ? '🔴'
            : status.trend === 'growing' ? '🟡'
            : '🟢';

        const trendLabel = status.trend === 'critical' ? 'CRITICAL — IDE Unstable'
            : status.trend === 'growing' ? 'WARNING — Dumps Accumulating'
            : 'STABLE';

        const extHostNote = status.lastExtHostCrash
            ? `<div class="card danger">
                 <h3>⚠️ Extension Host Crash Detected</h3>
                 <p>Last Extension Host crash: <strong>${status.lastExtHostCrash.toLocaleString()}</strong></p>
                 <p>This is the most dangerous crash type — it causes the V8 OOM → Jetsam cascade
                    documented in <a href="https://github.com/SirsiMaster/sirsi-pantheon/blob/main/docs/case-studies/session-23-extension-host-crash-forensics.md">Case Study 011</a>.</p>
               </div>`
            : '';

        return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Crashpad Stability Report</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
            background: #0F0F0F;
            color: #E5E5E5;
            padding: 2rem;
            line-height: 1.7;
        }
        h1 { color: #C8A951; font-size: 1.6rem; margin-bottom: 0.5rem; }
        h2 { color: #C8A951; font-size: 1.1rem; margin-top: 2rem; border-bottom: 1px solid #333; padding-bottom: 0.5rem; }
        h3 { color: #C8A951; font-size: 1rem; margin-bottom: 0.5rem; }
        .subtitle { color: #888; font-size: 0.85rem; }
        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 1rem;
            margin: 1.5rem 0;
        }
        .stat-card {
            background: rgba(200,169,81,0.06);
            border: 1px solid rgba(200,169,81,0.15);
            border-radius: 8px;
            padding: 1.2rem;
            text-align: center;
        }
        .stat-value {
            font-size: 2rem;
            font-weight: 700;
            color: #C8A951;
        }
        .stat-label {
            font-size: 0.75rem;
            color: #888;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            margin-top: 0.3rem;
        }
        .card {
            background: rgba(255,255,255,0.03);
            border: 1px solid rgba(255,255,255,0.08);
            border-radius: 8px;
            padding: 1.2rem;
            margin: 1rem 0;
        }
        .card.danger {
            border-color: rgba(220,38,38,0.4);
            background: rgba(220,38,38,0.06);
        }
        .card.warning {
            border-color: rgba(245,158,11,0.4);
            background: rgba(245,158,11,0.06);
        }
        .card.success {
            border-color: rgba(16,185,129,0.4);
            background: rgba(16,185,129,0.06);
        }
        .trend-badge {
            display: inline-block;
            padding: 4px 12px;
            border-radius: 99px;
            font-size: 0.8rem;
            font-weight: 600;
            letter-spacing: 0.05em;
        }
        .trend-critical { background: rgba(220,38,38,0.15); color: #ef4444; }
        .trend-growing { background: rgba(245,158,11,0.15); color: #f59e0b; }
        .trend-stable { background: rgba(16,185,129,0.15); color: #10B981; }
        code {
            font-family: 'JetBrains Mono', 'SF Mono', monospace;
            font-size: 0.82rem;
            background: rgba(255,255,255,0.06);
            padding: 2px 6px;
            border-radius: 4px;
        }
        .path { color: #888; font-size: 0.82rem; word-break: break-all; }
        .recommendation {
            margin-top: 2rem;
            padding: 1.2rem;
            border-left: 3px solid #C8A951;
            background: rgba(200,169,81,0.04);
        }
        a { color: #C8A951; }
    </style>
</head>
<body>
    <h1>𓁵 Crashpad Stability Report</h1>
    <p class="subtitle">Generated ${new Date().toLocaleString()} by Pantheon Guardian</p>

    <div class="stats">
        <div class="stat-card">
            <div class="stat-value">${status.pendingCount}</div>
            <div class="stat-label">Pending Dumps</div>
        </div>
        <div class="stat-card">
            <div class="stat-value">${status.recentCount}</div>
            <div class="stat-label">Last 24 Hours</div>
        </div>
        <div class="stat-card">
            <div class="stat-value">${trendIcon}</div>
            <div class="stat-label">Trend</div>
        </div>
    </div>

    <p><span class="trend-badge trend-${status.trend}">${trendLabel}</span></p>

    ${extHostNote}

    <h2>Timeline</h2>
    <div class="card">
        <p><strong>Oldest dump:</strong> ${status.oldestDump ? status.oldestDump.toLocaleString() : 'None'}</p>
        <p><strong>Newest dump:</strong> ${status.newestDump ? status.newestDump.toLocaleString() : 'None'}</p>
        <p><strong>Crashpad path:</strong> <span class="path">${status.crashpadPath}</span></p>
    </div>

    <h2>What This Means</h2>
    ${status.trend === 'critical'
        ? `<div class="card danger">
             <h3>🔴 IDE Stability at Risk</h3>
             <p>${status.pendingCount} crash dumps have accumulated without being submitted.
                This indicates repeated process crashes — potentially the Extension Host OOM pattern
                documented in Case Study 011.</p>
             <p>Consider clearing old dumps and monitoring for new ones. If the IDE feels sluggish
                or requires frequent restarts, the Extension Host may be chronically unstable.</p>
           </div>`
        : status.trend === 'growing'
        ? `<div class="card warning">
             <h3>🟡 Dumps Accumulating</h3>
             <p>The crash dump count is increasing. This is normal during active development
                if the IDE occasionally restarts, but sustained growth indicates a problem.</p>
           </div>`
        : `<div class="card success">
             <h3>🟢 Stable</h3>
             <p>Crash dump count is low and not growing. The IDE is operating normally.</p>
           </div>`
    }

    <h2>Recommended Actions</h2>
    <div class="recommendation">
        ${status.pendingCount > 10
        ? `<p><strong>1. Clear stale dumps:</strong></p>
           <p><code>rm ~/Library/Application\\ Support/Antigravity/Crashpad/pending/*.dmp</code></p>
           <p><strong>2. Monitor for new crashes:</strong> After clearing, if new dumps appear quickly,
              the IDE has a chronic issue. Check which extensions are active and consider disabling
              third-party extensions one by one.</p>`
        : `<p>No immediate action needed. Guardian will continue monitoring and alert you if the
              trend changes.</p>`
        }
    </div>

    <h2>Forensics Reference</h2>
    <div class="card">
        <p>Extract crash type from a dump:</p>
        <p><code>strings &lt;dump_file&gt; | grep "VSCODE_CRASH_REPORTER_PROCESS_TYPE"</code></p>
        <p>Check for V8 OOM:</p>
        <p><code>strings &lt;dump_file&gt; | grep "electron.v8-oom"</code></p>
        <p>Check for memory pressure kill:</p>
        <p><code>strings &lt;dump_file&gt; | grep "libMemoryResourceException"</code></p>
    </div>
</body>
</html>`;
    }

    // ── Clear Pending Dumps ───────────────────────────────────────

    async clearPendingDumps(): Promise<void> {
        const pendingDir = path.join(this.crashpadPath, 'pending');
        if (!fs.existsSync(pendingDir)) { return; }

        const confirm = await vscode.window.showWarningMessage(
            `This will delete all ${this.lastStatus?.pendingCount || 0} pending crash dumps. ` +
            `These are unsubmitted crash reports — they have no diagnostic value unless extracted manually. Continue?`,
            { modal: true },
            'Delete All'
        );

        if (confirm !== 'Delete All') { return; }

        try {
            const entries = fs.readdirSync(pendingDir).filter(f => f.endsWith('.dmp'));
            let deleted = 0;
            for (const entry of entries) {
                try {
                    fs.unlinkSync(path.join(pendingDir, entry));
                    deleted++;
                } catch {
                    // Skip files we can't delete
                }
            }
            this.output.appendLine(`𓁵 Crashpad: Cleared ${deleted} pending dumps`);
            this.history = [];
            vscode.window.showInformationMessage(
                `𓁵 Cleared ${deleted} crash dumps. Guardian will monitor for new crashes.`
            );
            // Re-check immediately
            await this.checkCrashpad();
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            this.output.appendLine(`𓁵 Crashpad clear error: ${msg}`);
            vscode.window.showErrorMessage(`Failed to clear crash dumps: ${msg}`);
        }
    }

    // ── Path Detection ────────────────────────────────────────────

    private detectCrashpadPath(): string {
        const home = os.homedir();

        // Antigravity IDE (primary target)
        const antigravityPath = path.join(home, 'Library', 'Application Support', 'Antigravity', 'Crashpad');
        if (fs.existsSync(antigravityPath)) {
            return antigravityPath;
        }

        // VS Code (fallback)
        const vscodePath = path.join(home, 'Library', 'Application Support', 'Code', 'Crashpad');
        if (fs.existsSync(vscodePath)) {
            return vscodePath;
        }

        // Cursor
        const cursorPath = path.join(home, 'Library', 'Application Support', 'Cursor', 'Crashpad');
        if (fs.existsSync(cursorPath)) {
            return cursorPath;
        }

        // Windsurf
        const windsurfPath = path.join(home, 'Library', 'Application Support', 'Windsurf', 'Crashpad');
        if (fs.existsSync(windsurfPath)) {
            return windsurfPath;
        }

        // Default to Antigravity even if it doesn't exist yet
        return antigravityPath;
    }

    // ── Public API ────────────────────────────────────────────────

    getStatus(): CrashpadStatus | null {
        return this.lastStatus;
    }
}
