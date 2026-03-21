// 𓂀 Sirsi Anubis — VS Code Extension (Scaffold)
//
// This is a scaffold for the Eye of Horus VS Code sidebar
// and workspace health monitoring. Full implementation will
// integrate with the Anubis MCP server for real-time data.
//
// Architecture:
//   extension.js → spawns `anubis mcp` → JSON-RPC 2.0 over stdio
//   Sidebar views pull from MCP resources
//   Commands invoke MCP tools

const vscode = require('vscode');

/**
 * @param {vscode.ExtensionContext} context
 */
function activate(context) {
    console.log('𓂀 Sirsi Anubis extension activated');

    // Register commands
    const scanCmd = vscode.commands.registerCommand('anubis.scanWorkspace', async () => {
        const terminal = vscode.window.createTerminal('Anubis Scan');
        terminal.sendText('anubis weigh --json');
        terminal.show();
    });

    const ghostCmd = vscode.commands.registerCommand('anubis.ghostReport', async () => {
        const terminal = vscode.window.createTerminal('Anubis Ka');
        terminal.sendText('anubis ka --json');
        terminal.show();
    });

    const healthCmd = vscode.commands.registerCommand('anubis.healthCheck', async () => {
        const terminal = vscode.window.createTerminal('Anubis Health');
        terminal.sendText('anubis hapi --gpu');
        terminal.show();
    });

    const brainCmd = vscode.commands.registerCommand('anubis.installBrain', async () => {
        const terminal = vscode.window.createTerminal('Anubis Brain');
        terminal.sendText('anubis install-brain');
        terminal.show();
    });

    // Status bar item — shows workspace health
    const statusBar = vscode.window.createStatusBarItem(
        vscode.StatusBarAlignment.Right,
        100
    );
    statusBar.text = '$(eye) Anubis';
    statusBar.tooltip = 'Sirsi Anubis — Infrastructure Hygiene';
    statusBar.command = 'anubis.healthCheck';
    statusBar.show();

    context.subscriptions.push(scanCmd, ghostCmd, healthCmd, brainCmd, statusBar);
}

function deactivate() {
    console.log('𓂀 Sirsi Anubis extension deactivated');
}

module.exports = {
    activate,
    deactivate
};
