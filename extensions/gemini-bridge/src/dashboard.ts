// 𓁆 Seshat — Webview Dashboard Panel
//
// A full-featured webview panel that shows the Knowledge Item
// inventory, Chrome profile selector, and six-direction sync status
// in the Pantheon gold-on-black Egyptian aesthetic.

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { SeshatPaths } from './paths';

export class SeshatDashboard {
    private panel: vscode.WebviewPanel | undefined;

    constructor(
        private context: vscode.ExtensionContext,
        private paths: SeshatPaths,
        private outputChannel: vscode.OutputChannel
    ) {}

    show(): void {
        if (this.panel) {
            this.panel.reveal(vscode.ViewColumn.One);
            this.update();
            return;
        }

        this.panel = vscode.window.createWebviewPanel(
            'seshatDashboard',
            '𓁆 Seshat — Gemini Bridge',
            vscode.ViewColumn.One,
            {
                enableScripts: true,
                retainContextWhenHidden: true,
            }
        );

        this.panel.onDidDispose(() => {
            this.panel = undefined;
        });

        this.panel.webview.onDidReceiveMessage(
            async (message: { command: string; payload?: string }) => {
                switch (message.command) {
                    case 'exportKI':
                        await vscode.commands.executeCommand('seshat.exportKI');
                        this.update();
                        break;
                    case 'syncKI':
                        await vscode.commands.executeCommand('seshat.syncToGemini');
                        this.update();
                        break;
                    case 'openKI':
                        if (message.payload) {
                            const metaPath = path.join(this.paths.knowledgeDir, message.payload, 'metadata.json');
                            if (fs.existsSync(metaPath)) {
                                const uri = vscode.Uri.file(metaPath);
                                await vscode.commands.executeCommand('vscode.open', uri);
                            }
                        }
                        break;
                    case 'refresh':
                        this.update();
                        break;
                }
            },
            undefined,
            this.context.subscriptions
        );

        this.update();
    }

    private update(): void {
        if (!this.panel) { return; }

        const kiData = this.getKnowledgeData();
        const conversationCount = this.getConversationCount();

        this.panel.webview.html = this.getWebviewContent(kiData, conversationCount);
    }

    private getKnowledgeData(): Array<{ name: string; title: string; summary: string; artifactCount: number }> {
        if (!fs.existsSync(this.paths.knowledgeDir)) { return []; }

        try {
            const entries = fs.readdirSync(this.paths.knowledgeDir, { withFileTypes: true });
            return entries
                .filter(e => e.isDirectory())
                .map(e => {
                    let title = e.name;
                    let summary = '';
                    let artifactCount = 0;

                    const metaPath = path.join(this.paths.knowledgeDir, e.name, 'metadata.json');
                    if (fs.existsSync(metaPath)) {
                        try {
                            const meta = JSON.parse(fs.readFileSync(metaPath, 'utf-8'));
                            title = meta.title || e.name;
                            summary = meta.summary || '';
                        } catch { /* fallback */ }
                    }

                    const artDir = path.join(this.paths.knowledgeDir, e.name, 'artifacts');
                    if (fs.existsSync(artDir)) {
                        try {
                            artifactCount = fs.readdirSync(artDir).length;
                        } catch { /* 0 */ }
                    }

                    return { name: e.name, title, summary: summary.slice(0, 120), artifactCount };
                });
        } catch {
            return [];
        }
    }

    private getConversationCount(): number {
        if (!fs.existsSync(this.paths.brainDir)) { return 0; }
        try {
            return fs.readdirSync(this.paths.brainDir, { withFileTypes: true })
                .filter(e => e.isDirectory() && e.name !== 'tempmediaStorage')
                .length;
        } catch {
            return 0;
        }
    }

    private getWebviewContent(
        kiData: Array<{ name: string; title: string; summary: string; artifactCount: number }>,
        conversationCount: number
    ): string {
        const kiRows = kiData.map(ki =>
            `<tr class="ki-row" onclick="openKI('${ki.name}')">
                <td class="ki-title">${this.escapeHtml(ki.title)}</td>
                <td class="ki-artifacts">${ki.artifactCount}</td>
                <td class="ki-summary">${this.escapeHtml(ki.summary)}${ki.summary.length >= 120 ? '…' : ''}</td>
            </tr>`
        ).join('\n');

        return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>𓁆 Seshat — Gemini Bridge</title>
    <style>
        :root {
            --bg: #0F0F0F;
            --surface: #1A1A1A;
            --surface-hover: #252525;
            --gold: #C5A03D;
            --gold-dim: #8B7228;
            --gold-bright: #E8C547;
            --text: #E0E0E0;
            --text-dim: #888;
            --green: #4CAF50;
            --red: #F44336;
            --blue: #42A5F5;
            --radius: 8px;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            background: var(--bg);
            color: var(--text);
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
            padding: 24px;
            line-height: 1.6;
        }

        .header {
            display: flex;
            align-items: center;
            gap: 16px;
            margin-bottom: 32px;
            padding-bottom: 16px;
            border-bottom: 2px solid var(--gold-dim);
        }

        .header-glyph {
            font-size: 48px;
            line-height: 1;
        }

        .header-text h1 {
            font-size: 28px;
            font-weight: 700;
            color: var(--gold);
            letter-spacing: 0.5px;
        }

        .header-text p {
            color: var(--text-dim);
            font-size: 14px;
        }

        /* Stats Cards */
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
            gap: 16px;
            margin-bottom: 32px;
        }

        .stat-card {
            background: var(--surface);
            border: 1px solid var(--gold-dim);
            border-radius: var(--radius);
            padding: 20px;
            text-align: center;
            transition: all 0.2s ease;
        }

        .stat-card:hover {
            border-color: var(--gold);
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(197, 160, 61, 0.15);
        }

        .stat-value {
            font-size: 36px;
            font-weight: 800;
            color: var(--gold-bright);
            display: block;
        }

        .stat-label {
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 1.5px;
            color: var(--text-dim);
            margin-top: 4px;
        }

        /* Directions */
        .directions-grid {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 12px;
            margin-bottom: 32px;
        }

        .direction-card {
            background: var(--surface);
            border: 1px solid #333;
            border-radius: var(--radius);
            padding: 14px;
            display: flex;
            align-items: center;
            gap: 10px;
            font-size: 13px;
        }

        .direction-num {
            background: var(--gold-dim);
            color: var(--bg);
            width: 24px;
            height: 24px;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            font-size: 12px;
            flex-shrink: 0;
        }

        /* Section Headers */
        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
        }

        .section-header h2 {
            font-size: 18px;
            color: var(--gold);
            font-weight: 600;
        }

        /* KI Table */
        .ki-table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 32px;
        }

        .ki-table th {
            text-align: left;
            padding: 10px 12px;
            background: var(--surface);
            color: var(--gold);
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 1px;
            border-bottom: 2px solid var(--gold-dim);
        }

        .ki-row {
            cursor: pointer;
            transition: background 0.15s ease;
        }

        .ki-row:hover {
            background: var(--surface-hover);
        }

        .ki-row td {
            padding: 10px 12px;
            border-bottom: 1px solid #222;
            font-size: 13px;
        }

        .ki-title { color: var(--text); font-weight: 500; }
        .ki-artifacts { color: var(--gold); text-align: center; }
        .ki-summary { color: var(--text-dim); max-width: 400px; }

        /* Buttons */
        .btn {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 8px 16px;
            border: 1px solid var(--gold-dim);
            background: transparent;
            color: var(--gold);
            border-radius: var(--radius);
            cursor: pointer;
            font-size: 13px;
            transition: all 0.2s ease;
        }

        .btn:hover {
            background: var(--gold-dim);
            color: var(--bg);
        }

        .btn-primary {
            background: var(--gold);
            color: var(--bg);
            border-color: var(--gold);
        }

        .btn-primary:hover {
            background: var(--gold-bright);
        }

        .actions {
            display: flex;
            gap: 12px;
        }

        .empty-state {
            text-align: center;
            padding: 48px;
            color: var(--text-dim);
        }

        .empty-state .glyph {
            font-size: 64px;
            margin-bottom: 16px;
        }
    </style>
</head>
<body>
    <div class="header">
        <span class="header-glyph">𓁆</span>
        <div class="header-text">
            <h1>Seshat — Gemini Bridge</h1>
            <p>Bidirectional knowledge sync across Gemini AI Mode, NotebookLM, and Antigravity IDE</p>
        </div>
    </div>

    <div class="stats-grid">
        <div class="stat-card">
            <span class="stat-value">${kiData.length}</span>
            <span class="stat-label">Knowledge Items</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">${conversationCount}</span>
            <span class="stat-label">Brain Conversations</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">6</span>
            <span class="stat-label">Bridge Directions</span>
        </div>
        <div class="stat-card">
            <span class="stat-value">${kiData.reduce((a, b) => a + b.artifactCount, 0)}</span>
            <span class="stat-label">Total Artifacts</span>
        </div>
    </div>

    <div class="section-header">
        <h2>Bridge Directions</h2>
    </div>
    <div class="directions-grid">
        <div class="direction-card"><span class="direction-num">1</span> Gemini → NotebookLM</div>
        <div class="direction-card"><span class="direction-num">2</span> NotebookLM → Gemini</div>
        <div class="direction-card"><span class="direction-num">3</span> NotebookLM → Antigravity</div>
        <div class="direction-card"><span class="direction-num">4</span> Antigravity → NotebookLM</div>
        <div class="direction-card"><span class="direction-num">5</span> Gemini → Antigravity</div>
        <div class="direction-card"><span class="direction-num">6</span> Antigravity → Gemini</div>
    </div>

    <div class="section-header">
        <h2>Knowledge Items</h2>
        <div class="actions">
            <button class="btn" onclick="refresh()">↻ Refresh</button>
            <button class="btn" onclick="exportKI()">⬆ Export</button>
            <button class="btn btn-primary" onclick="syncKI()">⇄ Sync to GEMINI.md</button>
        </div>
    </div>

    ${kiData.length > 0 ? `
    <table class="ki-table">
        <thead>
            <tr>
                <th>Title</th>
                <th>Artifacts</th>
                <th>Summary</th>
            </tr>
        </thead>
        <tbody>
            ${kiRows}
        </tbody>
    </table>
    ` : `
    <div class="empty-state">
        <div class="glyph">𓁆</div>
        <p>No Knowledge Items found.<br>Use the Gemini Bridge pipeline to extract and inject knowledge.</p>
    </div>
    `}

    <script>
        const vscode = acquireVsCodeApi();

        function openKI(name) {
            vscode.postMessage({ command: 'openKI', payload: name });
        }

        function exportKI() {
            vscode.postMessage({ command: 'exportKI' });
        }

        function syncKI() {
            vscode.postMessage({ command: 'syncKI' });
        }

        function refresh() {
            vscode.postMessage({ command: 'refresh' });
        }
    </script>
</body>
</html>`;
    }

    private escapeHtml(text: string): string {
        return text
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }
}
