// 𓁆 Seshat — Chrome Profiles Tree View Provider
//
// Discovers all Chrome profiles on the system and displays them
// in the sidebar, highlighting the active/default profile.

import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import * as os from 'os';

interface ChromeProfile {
    dirName: string;
    displayName: string;
    email: string;
    isDefault: boolean;
}

export class ChromeProfilesProvider implements vscode.TreeDataProvider<ChromeProfileNode> {
    private _onDidChangeTreeData = new vscode.EventEmitter<ChromeProfileNode | undefined>();
    readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

    constructor(private outputChannel: vscode.OutputChannel) {}

    refresh(): void {
        this._onDidChangeTreeData.fire(undefined);
    }

    getTreeItem(element: ChromeProfileNode): vscode.TreeItem {
        return element;
    }

    getChildren(): Thenable<ChromeProfileNode[]> {
        return this.discoverProfiles();
    }

    private async discoverProfiles(): Promise<ChromeProfileNode[]> {
        const chromeDir = this.getChromeUserDataDir();
        if (!chromeDir || !fs.existsSync(chromeDir)) {
            this.outputChannel.appendLine('𓁆 Chrome user data directory not found');
            return [new ChromeProfileNode('Chrome not detected', '', '', false)];
        }

        const config = vscode.workspace.getConfiguration('seshat');
        const defaultProfile = config.get<string>('defaultProfile', 'SirsiMaster');

        try {
            const localStatePath = path.join(chromeDir, 'Local State');
            if (!fs.existsSync(localStatePath)) {
                return [new ChromeProfileNode('No Local State file', '', '', false)];
            }

            const localState = JSON.parse(fs.readFileSync(localStatePath, 'utf-8'));
            const profileInfo = localState?.profile?.info_cache || {};

            const profiles: ChromeProfileNode[] = [];

            for (const [dirName, info] of Object.entries(profileInfo)) {
                const profileData = info as Record<string, unknown>;
                const displayName = String(profileData.name || dirName);
                const email = String(profileData.user_name || '');
                const isDefault = displayName === defaultProfile ||
                                  dirName === defaultProfile;

                const node = new ChromeProfileNode(displayName, dirName, email, isDefault);

                if (isDefault) {
                    node.iconPath = new vscode.ThemeIcon('star-full', new vscode.ThemeColor('charts.yellow'));
                    node.description = `★ Default — ${email}`;
                } else {
                    node.iconPath = new vscode.ThemeIcon('account');
                    node.description = email;
                }

                profiles.push(node);
            }

            // Sort: default first, then alphabetical
            profiles.sort((a, b) => {
                if (a.isDefault && !b.isDefault) { return -1; }
                if (!a.isDefault && b.isDefault) { return 1; }
                return a.label.localeCompare(b.label);
            });

            this.outputChannel.appendLine(`𓁆 Discovered ${profiles.length} Chrome profiles`);
            return profiles;
        } catch (err) {
            const msg = err instanceof Error ? err.message : String(err);
            this.outputChannel.appendLine(`𓁆 Error discovering profiles: ${msg}`);
            return [new ChromeProfileNode('Error reading profiles', '', msg, false)];
        }
    }

    private getChromeUserDataDir(): string | null {
        const platform = os.platform();
        const home = os.homedir();

        switch (platform) {
            case 'darwin':
                return path.join(home, 'Library', 'Application Support', 'Google', 'Chrome');
            case 'linux':
                return path.join(home, '.config', 'google-chrome');
            case 'win32':
                return path.join(home, 'AppData', 'Local', 'Google', 'Chrome', 'User Data');
            default:
                return null;
        }
    }

    /** Returns all discovered profile directory names. */
    getProfileNames(): string[] {
        const chromeDir = this.getChromeUserDataDir();
        if (!chromeDir || !fs.existsSync(chromeDir)) { return []; }

        try {
            const localStatePath = path.join(chromeDir, 'Local State');
            if (!fs.existsSync(localStatePath)) { return []; }

            const localState = JSON.parse(fs.readFileSync(localStatePath, 'utf-8'));
            const profileInfo = localState?.profile?.info_cache || {};
            return Object.keys(profileInfo);
        } catch {
            return [];
        }
    }
}

export class ChromeProfileNode extends vscode.TreeItem {
    constructor(
        public readonly label: string,
        public readonly dirName: string,
        public readonly email: string,
        public readonly isDefault: boolean
    ) {
        super(label, vscode.TreeItemCollapsibleState.None);
        this.contextValue = isDefault ? 'chromeProfile-default' : 'chromeProfile';
        this.tooltip = `Profile: ${label}\nDirectory: ${dirName}\nEmail: ${email}`;
    }
}
