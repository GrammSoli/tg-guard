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

// ─── Init ──────────────────────────────────────────────────────

export function initTelegramApp() {
  try {
    const wa = tg();
    if (!wa) return;
    (wa.ready as (() => void) | undefined)?.();
    (wa.expand as (() => void) | undefined)?.();
  } catch { /* noop */ }
}
