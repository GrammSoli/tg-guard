/**
 * Track the visible viewport and expose it to CSS as custom properties
 * on documentElement:
 *
 *   --app-vh   — visible viewport height in px (shrinks with keyboard)
 *   --app-vw   — visible viewport width in px
 *   --kb-inset — keyboard inset in px (0 when no keyboard)
 *
 * Why two sources:
 *   - `window.visualViewport.height` is the W3C-standard signal and works
 *     reliably in iOS Safari / Chromium. However, on iOS Telegram the
 *     keyboard sometimes floats over the WebView WITHOUT shrinking it,
 *     so visualViewport reports the full height even with the keyboard
 *     open.
 *   - `Telegram.WebApp.viewportHeight` is the Telegram-reported visible
 *     height. It DOES shrink with the keyboard inside the Telegram app
 *     (paired with a `viewportChanged` event we listen to).
 *
 * Taking `Math.min(visualVh, telegramVh)` means whichever signal sees the
 * keyboard wins — the drawer ends up anchored above the keyboard either
 * way. `kb-inset = max(0, window.innerHeight - effectiveVh)` is then the
 * amount we need to shift bottom-anchored sheets up.
 *
 * Components read from CSS — no React state, no re-renders. A bottom
 * sheet becomes `bottom: var(--kb-inset); max-height: calc(var(--app-vh) * 0.85)`
 * and Just Works.
 *
 * Call once before render in main.tsx.
 */

interface TelegramWebApp {
  viewportHeight?: number;
  viewportStableHeight?: number;
  onEvent?: (event: string, handler: () => void) => void;
}

function getTelegramWebApp(): TelegramWebApp | null {
  if (typeof window === "undefined") return null;
  const tg = (window as unknown as { Telegram?: { WebApp?: TelegramWebApp } }).Telegram;
  return tg?.WebApp ?? null;
}

export function initViewportTracking() {
  if (typeof window === "undefined") return;
  const root = document.documentElement;
  const vv = window.visualViewport;
  const tg = getTelegramWebApp();

  const sync = () => {
    const layoutVh = window.innerHeight;
    const visualVh = vv?.height ?? layoutVh;
    // Telegram only reports a positive viewportHeight after the WebApp
    // is ready; fall back to layoutVh until then.
    const telegramVh =
      typeof tg?.viewportHeight === "number" && tg.viewportHeight > 0
        ? tg.viewportHeight
        : layoutVh;

    // Whichever signal sees the keyboard (returns a smaller height) wins.
    const effectiveVh = Math.min(visualVh, telegramVh);
    const inset = Math.max(0, layoutVh - effectiveVh);
    const w = vv?.width ?? window.innerWidth;

    root.style.setProperty("--app-vh", `${effectiveVh}px`);
    root.style.setProperty("--app-vw", `${w}px`);
    root.style.setProperty("--kb-inset", `${inset}px`);
  };

  sync();

  if (vv) {
    vv.addEventListener("resize", sync);
    vv.addEventListener("scroll", sync);
  }
  window.addEventListener("resize", sync);

  // Telegram fires viewportChanged on keyboard show/hide inside its
  // WebView. Critical on iOS where the keyboard floats over the
  // WebView and visualViewport doesn't shrink.
  if (tg?.onEvent) {
    try {
      tg.onEvent("viewportChanged", sync);
    } catch {
      /* noop — older Telegram clients may not support onEvent */
    }
  }
}
