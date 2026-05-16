/**
 * Telegram Mini App integration utilities.
 *
 * All functions are safe to call outside Telegram — they silently no-op
 * when `window.Telegram` is unavailable (e.g. in a regular browser).
 */

/** Whether we're running inside Telegram WebView */
export function isTelegramWebApp(): boolean {
  if (typeof window === "undefined") return false;
  return !!((window as unknown as Record<string, unknown>).Telegram);
}

/** Access the raw Telegram.WebApp object if available */
function tg() {
  if (typeof window === "undefined") return null;
  try {
    return (window as unknown as { Telegram?: { WebApp?: Record<string, unknown> } })
      ?.Telegram?.WebApp ?? null;
  } catch {
    return null;
  }
}

// ─── Haptic Feedback ───────────────────────────────────────────

export type HapticImpact = "light" | "medium" | "heavy" | "rigid" | "soft";
export type HapticNotification = "error" | "success" | "warning";

export function hapticImpact(style: HapticImpact = "light") {
  try {
    const hf = tg()?.HapticFeedback as
      | { impactOccurred?: (s: string) => void }
      | undefined;
    hf?.impactOccurred?.(style);
  } catch { /* noop outside TMA */ }
}

export function hapticNotification(type: HapticNotification = "success") {
  try {
    const hf = tg()?.HapticFeedback as
      | { notificationOccurred?: (t: string) => void }
      | undefined;
    hf?.notificationOccurred?.(type);
  } catch { /* noop */ }
}

export function hapticSelection() {
  try {
    const hf = tg()?.HapticFeedback as
      | { selectionChanged?: () => void }
      | undefined;
    hf?.selectionChanged?.();
  } catch { /* noop */ }
}

// ─── Back Button ───────────────────────────────────────────────

function backButton() {
  return tg()?.BackButton as
    | {
        show?: () => void;
        hide?: () => void;
        onClick?: (cb: () => void) => void;
        offClick?: (cb: () => void) => void;
      }
    | undefined;
}

// Stack of registered back-button handlers. We swap which one is bound to
// the Telegram BackButton as components mount/unmount, so nested sheets
// (e.g. SharedRoomSheet over a route) restore the parent's handler on close.
const _backStack: Array<() => void> = [];

function bindTop(bb: ReturnType<typeof backButton>) {
  if (!bb) return;
  const top = _backStack[_backStack.length - 1];
  if (top) {
    bb.onClick?.(top);
    bb.show?.();
  } else {
    bb.hide?.();
  }
}

export function showBackButton(onClick: () => void) {
  try {
    const bb = backButton();
    if (!bb) return;
    // Detach whatever was on top so only one handler is bound at a time.
    const prev = _backStack[_backStack.length - 1];
    if (prev && bb.offClick) bb.offClick(prev);
    _backStack.push(onClick);
    bindTop(bb);
  } catch { /* noop */ }
}

export function hideBackButton() {
  try {
    const bb = backButton();
    if (!bb) return;
    const top = _backStack.pop();
    if (top && bb.offClick) bb.offClick(top);
    bindTop(bb);
  } catch { /* noop */ }
}

// ─── Close / Expand ────────────────────────────────────────────

export function closeMiniApp() {
  try {
    (tg()?.close as (() => void) | undefined)?.();
  } catch { /* noop */ }
}

export function expandMiniApp() {
  try {
    (tg()?.expand as (() => void) | undefined)?.();
  } catch { /* noop */ }
}

// ─── External links ────────────────────────────────────────────

/**
 * openExternalLink — open a URL outside the mini-app.
 * Prefers Telegram.WebApp.openLink (which opens the user's default browser
 * in-app) and falls back to window.open in non-TMA contexts.
 */
export function openExternalLink(url: string) {
  try {
    const openLink = tg()?.openLink as ((u: string) => void) | undefined;
    if (openLink) {
      openLink(url);
      return;
    }
  } catch { /* noop */ }
  if (typeof window !== "undefined") {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

/**
 * openTelegramLink — open a t.me/... link inside Telegram.
 * Used for Crypto Pay (@CryptoBot) invoice links which are t.me/CryptoBot
 * URLs. Unlike openExternalLink, this keeps the user inside Telegram.
 */
export function openTelegramLink(url: string) {
  try {
    const fn = tg()?.openTelegramLink as ((u: string) => void) | undefined;
    if (fn) {
      fn(url);
      return;
    }
  } catch { /* noop */ }
  // Fallback for non-TMA contexts
  if (typeof window !== "undefined") {
    window.open(url, "_blank", "noopener,noreferrer");
  }
}

// ─── User Info ─────────────────────────────────────────────────

export interface TelegramUser {
  id: number;
  first_name: string;
  last_name?: string;
  username?: string;
  language_code?: string;
  photo_url?: string;
}

export function getTelegramUser(): TelegramUser | null {
  try {
    const initData = tg()?.initDataUnsafe as
      | { user?: TelegramUser }
      | undefined;
    return initData?.user ?? null;
  } catch {
    return null;
  }
}

// ─── Payments (Stars) ──────────────────────────────────────────

/**
 * openInvoice — opens Telegram's native payment sheet for a Stars invoice.
 *
 * Returns a Promise that resolves with the payment status string:
 *   "paid"      — user completed the payment
 *   "cancelled" — user closed the payment sheet
 *   "failed"    — payment failed
 *   "pending"   — payment is still processing
 *
 * Falls back to rejecting with an Error when running outside Telegram.
 *
 * Timeout: if Telegram never invokes the callback (lib bug, WebView
 * killed mid-payment, ancient client) the Promise rejects after 5
 * minutes so callers don't have their `loading` UI state stuck on
 * forever. 5 minutes is well over the typical payment-sheet
 * completion time including card entry, so legitimate slow users
 * still resolve normally. Audit Tier-4 #3.
 */
const openInvoiceTimeoutMs = 5 * 60 * 1000;

export function openInvoice(url: string): Promise<string> {
  return new Promise((resolve, reject) => {
    try {
      const wa = tg();
      if (!wa) {
        reject(new Error("Not running inside Telegram"));
        return;
      }
      const openInvoiceFn = wa.openInvoice as
        | ((url: string, cb: (status: string) => void) => void)
        | undefined;
      if (!openInvoiceFn) {
        reject(new Error("openInvoice not available"));
        return;
      }
      let settled = false;
      const timer = window.setTimeout(() => {
        if (settled) return;
        settled = true;
        reject(new Error("openInvoice timeout — Telegram did not return a status"));
      }, openInvoiceTimeoutMs);
      openInvoiceFn(url, (status: string) => {
        if (settled) return;
        settled = true;
        window.clearTimeout(timer);
        resolve(status);
      });
    } catch (err) {
      reject(err);
    }
  });
}

// ─── Init ──────────────────────────────────────────────────────

export function initTelegramApp() {
  try {
    const wa = tg();
    if (!wa) return;
    (wa.ready as (() => void) | undefined)?.();
    (wa.expand as (() => void) | undefined)?.();
  } catch { /* noop */ }
}
