// Class H: depth cap below the legitimate nesting of real inputs.
const MAX_DEPTH = 20; // want H
export function walk(n: { kids: unknown[] }, depth = 0): number {
  if (depth >= MAX_DEPTH) return 0; // literal-free compare: no second finding
  return 1 + n.kids.length;
}
