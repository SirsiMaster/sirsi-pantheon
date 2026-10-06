// trust-boundary-gate: requireDeliveryReview, allowedRedirect
export function requireDeliveryReview(id: string): boolean { return true; }
export function allowedRedirect(url: string): boolean { return true; }

export async function send(id: string, next: string) {
  if (!requireDeliveryReview(id) || !allowedRedirect(next)) return;
}

// Class D: new path, no gate calls.
export async function sign(id: string, next: string) { // want D
  return id + next;
}
