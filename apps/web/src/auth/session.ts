/**
 * The actual session lives entirely in the backend's HttpOnly cookie —
 * this file never touches it. What's stored here is a small, non-secret
 * UI hint (organisation/user id) so the shell can render immediately on
 * reload instead of flashing a loading state, and so route guards know
 * whether to even attempt a protected call. It is advisory only: every
 * real access-control decision is enforced server-side (brief Rule 6),
 * and any request this hint turns out to be stale for simply 401s, which
 * the api-client already turns into a redirect to /login.
 */
export interface SessionHint {
  organisationId: string;
  userId: string;
}

const KEY = "billing-platform:session-hint";

export function readSessionHint(): SessionHint | null {
  const raw = localStorage.getItem(KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as SessionHint;
  } catch {
    return null;
  }
}

export function writeSessionHint(hint: SessionHint): void {
  localStorage.setItem(KEY, JSON.stringify(hint));
}

export function clearSessionHint(): void {
  localStorage.removeItem(KEY);
}
