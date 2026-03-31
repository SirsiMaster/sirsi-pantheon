#!/usr/bin/env node
// 𓁟 thoth-init — Initialize Thoth knowledge system in any project
// MIT License — Sirsi Technologies
// Usage: npx thoth-init

const fs = require('fs');
const path = require('path');
const readline = require('readline');

const THOTH_DIR = '.thoth';
const ARTIFACTS_DIR = path.join(THOTH_DIR, 'artifacts');

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

function ask(question, defaultVal) {
  return new Promise(resolve => {
    const prompt = defaultVal ? `${question} [${defaultVal}]: ` : `${question}: `;
    rl.question(prompt, answer => resolve(answer.trim() || defaultVal || ''));
  });
}

function detectProject() {
  const info = { name: path.basename(process.cwd()), language: 'unknown', version: '0.1.0' };
  
  // package.json
  if (fs.existsSync('package.json')) {
    try {
      const pkg = JSON.parse(fs.readFileSync('package.json', 'utf8'));
      info.name = pkg.name || info.name;
      info.version = pkg.version || info.version;
      info.language = 'TypeScript/JavaScript';
      if (fs.existsSync('next.config.js') || fs.existsSync('next.config.ts') || fs.existsSync('next.config.mjs'))
        info.language = 'TypeScript (Next.js)';
      else if (fs.existsSync('vite.config.ts') || fs.existsSync('vite.config.js'))
        info.language = 'TypeScript (Vite)';
    } catch {}
  }
  
  // go.mod
  if (fs.existsSync('go.mod')) {
    info.language = 'Go';
    try {
      const mod = fs.readFileSync('go.mod', 'utf8');
      const match = mod.match(/^module\s+(.+)/m);
      if (match) info.name = match[1].split('/').pop();
    } catch {}
  }
  
  // Cargo.toml
  if (fs.existsSync('Cargo.toml')) {
    info.language = 'Rust';
    try {
      const cargo = fs.readFileSync('Cargo.toml', 'utf8');
      const match = cargo.match(/name\s*=\s*"(.+?)"/);
      if (match) info.name = match[1];
    } catch {}
  }
  
  // pyproject.toml or setup.py
  if (fs.existsSync('pyproject.toml') || fs.existsSync('setup.py'))
    info.language = 'Python';
  
  return info;
}

function countSourceLines() {
  const extensions = ['.go', '.ts', '.tsx', '.js', '.jsx', '.py', '.rs', '.java', '.swift'];
  const excludeDirs = ['node_modules', '.next', 'dist', 'build', 'vendor', '__pycache__', '.git'];
  let total = 0;
  
  function walk(dir, depth) {
    if (depth > 8) return;
    try {
      const entries = fs.readdirSync(dir, { withFileTypes: true });
      for (const entry of entries) {
        if (excludeDirs.includes(entry.name)) continue;
        const full = path.join(dir, entry.name);
        if (entry.isDirectory()) {
          walk(full, depth + 1);
        } else if (extensions.some(ext => entry.name.endsWith(ext))) {
          try {
            const content = fs.readFileSync(full, 'utf8');
            total += content.split('\n').length;
          } catch {}
        }
      }
    } catch {}
  }
  
  walk('.', 0);
  return total;
}

function scanArchitecture() {
  const dirs = [];
  try {
    const entries = fs.readdirSync('.', { withFileTypes: true });
    for (const e of entries) {
      if (e.isDirectory() && !e.name.startsWith('.') && !['node_modules', 'dist', 'build', '.next'].includes(e.name)) {
        dirs.push(e.name);
      }
    }
  } catch {}
  return dirs;
}

async function main() {
  const nonInteractive = process.argv.includes('--yes') || process.argv.includes('-y');
  
  console.log('\n  𓁟 Thoth — Persistent Knowledge for AI-Assisted Development\n');
  
  if (fs.existsSync(path.join(THOTH_DIR, 'memory.yaml'))) {
    if (!nonInteractive) {
      console.log('  ⚠ .thoth/memory.yaml already exists.');
      const overwrite = await ask('  Overwrite? (y/N)', 'n');
      if (overwrite.toLowerCase() !== 'y') {
        console.log('  Aborted.\n');
        rl.close();
        return;
      }
    }
  }
  
  // Auto-detect
  console.log('  Scanning project...');
  const info = detectProject();
  const lineCount = countSourceLines();
  const topDirs = scanArchitecture();
  
  console.log(`  Detected: ${info.language} project "${info.name}" v${info.version}`);
  console.log(`  Source: ~${lineCount.toLocaleString()} lines\n`);
  
  let name, lang, version;
  if (nonInteractive) {
    name = info.name;
    lang = info.language;
    version = info.version;
  } else {
    name = await ask('  Project name', info.name);
    lang = await ask('  Language', info.language);
    version = await ask('  Version', info.version);
  }
  
  // Create structure
  fs.mkdirSync(ARTIFACTS_DIR, { recursive: true });
  
  const now = new Date().toISOString();
  
  // Memory file
  let memory = `# 𓁟 Thoth — Project Memory
# Read this FIRST before any source files.
# Last updated: ${now}

## Identity
project: ${name}
language: ${lang}
version: ${version}

## Stats
line_count: ~${lineCount.toLocaleString()}

## Architecture Quick Reference
`;
  
  for (const dir of topDirs) {
    memory += `# ${dir}/  — TODO: describe\n`;
  }
  
  memory += `
## Critical Design Decisions
# TODO: Add your key technical choices and rationale

## Known Limitations
# TODO: What doesn't work, what's incomplete

## Recent Changes
# ${now.split('T')[0]}: Thoth initialized

## File Map
# .thoth/memory.yaml  — THIS FILE
# .thoth/journal.md   — engineering journal
`;
  
  fs.writeFileSync(path.join(THOTH_DIR, 'memory.yaml'), memory);
  
  // Journal
  const journal = `# 𓁟 Engineering Journal
# Timestamped reasoning — the WHY behind every decision.

---

## Entry 001 — ${now.split('T')[0]} — Thoth Initialized

**Context**: Thoth knowledge system initialized for ${name}.

**Decision**: Three-layer knowledge system (memory → journal → artifacts) adopted to give AI assistants persistent context across sessions. ~${lineCount.toLocaleString()} source lines compressed to ~100 lines of structured YAML.

---
`;
  
  fs.writeFileSync(path.join(THOTH_DIR, 'journal.md'), journal);
  
  // Artifacts README
  fs.writeFileSync(path.join(ARTIFACTS_DIR, 'README.md'),
    '# Thoth Artifacts\nBenchmarks, audits, design docs, and deep analysis go here.\n');
  
  // ─── IDE Integration: inject "read .thoth/memory.yaml" into rules files ───
  const thothRule = `
# 𓁟 Thoth — Persistent Knowledge System
# At the START of every conversation, read .thoth/memory.yaml before reading any source files.
# This file is the project's compressed state (~100 lines) and replaces reading thousands of lines of code.
# When reasoning matters, also read .thoth/journal.md for decision history.
# After making significant changes, update both files.
`;

  const ideFiles = [
    { path: '.cursorrules',           name: 'Cursor' },
    { path: '.windsurfrules',         name: 'Windsurf' },
    { path: '.gemini/style.md',       name: 'Gemini', dir: '.gemini' },
    { path: 'CLAUDE.md',             name: 'Claude Code' },
    { path: '.github/copilot-instructions.md', name: 'Copilot', dir: '.github' },
  ];

  const injectedIDEs = [];

  for (const ide of ideFiles) {
    const exists = fs.existsSync(ide.path);
    if (exists) {
      // Check if already injected
      const content = fs.readFileSync(ide.path, 'utf8');
      if (content.includes('.thoth/memory.yaml')) {
        continue; // Already has Thoth
      }
      // Append to existing file
      fs.appendFileSync(ide.path, '\n' + thothRule);
      injectedIDEs.push(ide.name + ' (appended)');
    } else {
      // Create new rules file with Thoth instruction
      if (ide.dir) fs.mkdirSync(ide.dir, { recursive: true });
      fs.writeFileSync(ide.path, thothRule.trim() + '\n');
      injectedIDEs.push(ide.name + ' (created)');
    }
  }

  // Also create/update session workflow for Claude Code / Antigravity
  const workflowDir = '.agent/workflows';
  const workflowPath = path.join(workflowDir, 'session-start.md');
  if (!fs.existsSync(workflowPath)) {
    fs.mkdirSync(workflowDir, { recursive: true });
    fs.writeFileSync(workflowPath, `---
description: How to start a new session using the Thoth knowledge system
---

# 𓁟 Thoth Session Start

## Step 1: Read the Thoth memory file (ALWAYS do this first)
Read \`.thoth/memory.yaml\` in the project root. This replaces reading source code.

## Step 2: Read the engineering journal (when reasoning matters)
Read \`.thoth/journal.md\` for timestamped decision entries.

## Step 3: After making significant changes — update Thoth
1. Update \`.thoth/memory.yaml\` with new stats, decisions, limitations
2. Add a journal entry to \`.thoth/journal.md\`
3. Commit Thoth files with your code changes
`);
    injectedIDEs.push('Claude Code workflow (created)');
  }

  // ─── Summary ───
  console.log('\n  ✓ Created .thoth/memory.yaml');
  console.log('  ✓ Created .thoth/journal.md');
  console.log('  ✓ Created .thoth/artifacts/');
  
  if (injectedIDEs.length > 0) {
    console.log('\n  IDE integrations:');
    for (const ide of injectedIDEs) {
      console.log(`  ✓ ${ide}`);
    }
  }
  
  const tokensSaved = Math.round((1 - 100 / Math.max(lineCount, 100)) * 100);
  console.log(`\n  𓁟 Context reduction: ~${tokensSaved}%`);
  console.log(`     ${lineCount.toLocaleString()} source lines → ~100 Thoth lines`);
  console.log(`\n  Next: Populate memory.yaml with your architecture and decisions.`);
  console.log(`  The AI will read this file FIRST instead of re-reading source code.\n`);
  
  rl.close();
}

main().catch(err => {
  console.error('Error:', err.message);
  rl.close();
  process.exit(1);
});
