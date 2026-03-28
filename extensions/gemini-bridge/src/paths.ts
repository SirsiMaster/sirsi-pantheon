// 𓁆 Seshat — Path resolution for Antigravity directories
//
// Mirrors the Go `seshat.DefaultPaths()` struct.

import * as path from 'path';
import * as os from 'os';

export interface SeshatPaths {
    antigravityDir: string;
    knowledgeDir: string;
    brainDir: string;
    conversationsDir: string;
    skillDir: string;
}

/**
 * Resolves the standard Antigravity filesystem paths.
 * @param override Optional override for the Antigravity base directory.
 */
export function resolvePaths(override?: string): SeshatPaths {
    const base = override && override.trim().length > 0
        ? override.trim()
        : path.join(os.homedir(), '.gemini', 'antigravity');

    return {
        antigravityDir: base,
        knowledgeDir: path.join(base, 'knowledge'),
        brainDir: path.join(base, 'brain'),
        conversationsDir: path.join(base, 'conversations'),
        skillDir: path.join(base, 'skills', 'gemini-bridge'),
    };
}
