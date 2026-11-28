// Class A: max-of-keys loop bound in web code.
export function rows(values: Record<string, string>): string[] {
  const max = Math.max(...Object.keys(values).map(Number));
  const out = Array.from({ length: max + 1 }, () => ""); // want A
  for (let i = 0; i <= max; i++) { // want A
    out[i] = values[String(i)] ?? "";
  }
  return out;
}
