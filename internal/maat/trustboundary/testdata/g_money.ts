// Class G: '-500' becomes '500'.
export function parseAmount(raw: string): number {
  const amountDigits = raw.replace(/\D/g, ""); // want G
  return Number(amountDigits);
}
export function parseZip(raw: string): string {
  return raw.replace(/\D/g, ""); // a postal code: digits only IS its meaning, no finding
}
