// 𓁆 Seshat — Knowledge Items Tree View Provider
//
// Reads the Antigravity knowledge directory and displays
// all Knowledge Items as tree nodes with metadata.

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { SeshatPaths } from './paths';

interface KIMetadata {
    title?: string;
    summary?: string;
    references?: Array<{ type: string; value: string }>;
}

export class KnowledgeItemsProvider implements vscode.TreeDataProvider<KnowledgeItemNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<KnowledgeItemNode | undefined>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(
        private paths: SeshatPaths,
        private outputChannel: vscode.OutputChannel
    ) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: KnowledgeItemNode): vscode.TreeItem {
        return element;
    }

    getChildren(element?: KnowledgeItemNode): Thenable<KnowledgeItemNode[]> {
        if (element) {
            return this.getArtifacts(element.dirName);
        }
        return this.getKnowledgeItems();
    }

    private async getKnowledgeItems(): Promise<KnowledgeItemNode[]> {
        if (!fs.existsSync(this.paths.knowledgeDir)) {
            this.outputChannel.appendLine(`𓁆 Knowledge directory not found: ${this.paths.knowledgeDir}`);
            return [new KnowledgeItemNode('No knowledge items found', '', vscode.TreeItemCollapsibleState.None)];
        }

        try {
            const entries = fs.readdirSync(this.paths.knowledgeDir, { withFileTypes: true });
            const items: KnowledgeItemNode[] = [];

            for (const entry of entries) {
                if (!entry.isDirectory()) { continue; }

                const metaPath = path.join(this.paths.knowledgeDir, entry.name, 'metadata.json');
                let title = entry.name;
                let summary = '';

                if (fs.existsSync(metaPath)) {
                    try {
                        const raw = fs.readFileSync(metaPath, 'utf-8');
                        const meta: KIMetadata = JSON.parse(raw);
                        title = meta.title || entry.name;
                        summary = meta.summary || '';
                    } catch {
                        // Use directory name as fallback
                    }
                }

                const artifactsDir = path.join(this.paths.knowledgeDir, entry.name, 'artifacts');
                let artifactCount = 0;
                if (fs.existsSync(artifactsDir)) {
                    artifactCount = fs.readdirSync(artifactsDir).length;
                }

                const node = new KnowledgeItemNode(
                    title,
                    entry.name,
                    artifactCount > 0
                        ? vscode.TreeItemCollapsibleState.Collapsed
                        : vscode.TreeItemCollapsibleState.None
                );
                node.description = `${artifactCount} artifact${artifactCount === 1 ? '' : 's'}`;
                node.tooltip = summary || `Knowledge Item: ${entry.name}`;
                node.iconPath = new vscode.ThemeIcon('library');
                node.contextValue = 'knowledgeItem';

                items.push(node);
            }

            this.outputChannel.appendLine(`𓁆 Loaded ${items.length} Knowledge Items`);
            return items;
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            this.outputChannel.appendLine(`𓁆 Error reading KIs: ${msg}`);
            return [];
        }
    }

    private async getArtifacts(kiName: string): Promise<KnowledgeItemNode[]> {
        const artifactsDir = path.join(this.paths.knowledgeDir, kiName, 'artifacts');
        if (!fs.existsSync(artifactsDir)) { return []; }

        try {
            const entries = fs.readdirSync(artifactsDir, { withFileTypes: true });
            return entries
                .filter(e => e.isFile() && e.name.endsWith('.md'))
                .map(e => {
                    const node = new KnowledgeItemNode(
                        e.name,
                        kiName,
                        vscode.TreeItemCollapsibleState.None
                    );
                    node.iconPath = new vscode.ThemeIcon('file-text');
                    node.command = {
                        command: 'vscode.open',
                        title: 'Open Artifact',
                        arguments: [vscode.Uri.file(path.join(artifactsDir, e.name))],
                    };
                    return node;
                });
        } catch {
            return [];
        }
    }

    /** Returns the list of KI directory names. */
    getKINames(): string[] {
        if (!fs.existsSync(this.paths.knowledgeDir)) { return []; }
        try {
            return fs.readdirSync(this.paths.knowledgeDir, { withFileTypes: true })
                .filter(e => e.isDirectory())
                .map(e => e.name);
        } catch {
            return [];
        }
    }
}

export class KnowledgeItemNode extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly dirName: string,
        public readonly collapsibleState: vscode.TreeItemCollapsibleState
    ) {
        super(label, collapsibleState);
    }
}
