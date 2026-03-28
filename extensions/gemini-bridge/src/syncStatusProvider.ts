// 𓁆 Seshat — Sync Status Tree View Provider
//
// Shows the current state of the six-direction bridge:
// which KIs have been synced, last sync timestamps,
// and the health of each pipeline direction.

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { SeshatPaths } from './paths';

interface ManifestEntry {
    direction: string;
    timestamp: string;
    status: string;
    itemCount: number;
}

export class SyncStatusProvider implements vscode.TreeDataProvider<SyncStatusNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<SyncStatusNode | undefined>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(
        private paths: SeshatPaths,
        private outputChannel: vscode.OutputChannel
    ) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: SyncStatusNode): vscode.TreeItem {
        return element;
    }

    getChildren(): Thenable<SyncStatusNode[]> {
        return this.getSyncStatus();
    }

    private async getSyncStatus(): Promise<SyncStatusNode[]> {
        const directions = [
            { id: '1', label: 'Gemini → NotebookLM', icon: 'arrow-right' },
            { id: '2', label: 'NotebookLM → Gemini', icon: 'arrow-left' },
            { id: '3', label: 'NotebookLM → Antigravity', icon: 'arrow-down' },
            { id: '4', label: 'Antigravity → NotebookLM', icon: 'arrow-up' },
            { id: '5', label: 'Gemini → Antigravity', icon: 'arrow-right' },
            { id: '6', label: 'Antigravity → Gemini', icon: 'arrow-up' },
        ];

        // Try to read manifest for last sync data
        const manifest = this.readManifest();

        return directions.map(dir => {
            const entry = manifest.find(m => m.direction === dir.id);
            const node = new SyncStatusNode(
                `Direction ${dir.id}: ${dir.label}`,
                vscode.TreeItemCollapsibleState.None
            );

            if (entry) {
                const date = new Date(entry.timestamp);
                const relTime = this.relativeTime(date);
                node.description = `${entry.status === 'success' ? '✓' : '⚠'} ${relTime} — ${entry.itemCount} items`;
                node.iconPath = new vscode.ThemeIcon(
                    entry.status === 'success' ? 'pass-filled' : 'warning',
                    entry.status === 'success'
                        ? new vscode.ThemeColor('charts.green')
                        : new vscode.ThemeColor('charts.yellow')
                );
            } else {
                node.description = 'Not synced';
                node.iconPath = new vscode.ThemeIcon('circle-outline');
            }

            node.tooltip = `${dir.label}\n${entry ? `Last: ${entry.timestamp}\nItems: ${entry.itemCount}` : 'No sync history'}`;
            return node;
        });
    }

    private readManifest(): ManifestEntry[] {
        const manifestPath = path.join(this.paths.skillDir, 'data', 'manifest.json');
        if (!fs.existsSync(manifestPath)) { return []; }

        try {
            const raw = fs.readFileSync(manifestPath, 'utf-8');
            const data = JSON.parse(raw);
            return Array.isArray(data.syncs) ? data.syncs : [];
        } catch {
            return [];
        }
    }

    private relativeTime(date: Date): string {
        const now = Date.now();
        const diffMs = now - date.getTime();
        const diffMin = Math.floor(diffMs / 60000);

        if (diffMin < 1) { return 'just now'; }
        if (diffMin < 60) { return `${diffMin}m ago`; }

        const diffHr = Math.floor(diffMin / 60);
        if (diffHr < 24) { return `${diffHr}h ago`; }

        const diffDay = Math.floor(diffHr / 24);
        return `${diffDay}d ago`;
    }
}

export class SyncStatusNode extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly collapsibleState: vscode.TreeItemCollapsibleState
    ) {
        super(label, collapsibleState);
    }
}
