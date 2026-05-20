/**
 * Track the visible viewport via the W3C VisualViewport API and expose
 * the result as CSS custom properties on documentElement:
 *
 *   --app-vh   — visible viewport height in px (shrinks with keyboard)
 *   --app-vw   — visible viewport width in px
 *   --kb-inset — keyboard inset in px (layoutVh - visualVh), 0 with no kb
 *
 * Why this exists:
 *   - `100dvh` and `interactive-widget=resizes-content` rely on browser
 *     support that isn't uniform across Telegram WebView versions, and
 *     `Telegram.WebApp.viewportStableHeight` has its own semantic quirks
 *     ("last stable state" — sometimes pre-keyboard, sometimes not).
 *   - `window.visualViewport.height` IS uniform: it's the actual visible
 *     area excluding the on-screen keyboard, on iOS 13+ and Chromium 61+
 *     (which covers every Telegram client in active use).
 *
 * Components that need to anchor to the keyboard read these variables
 * directly from CSS — no React state, no re-renders. A bottom sheet
 * becomes `bottom: var(--kb-inset); max-height: calc(var(--app-vh) * 0.85)`
 * and Just Works.
 *
 * Call once before render in main.tsx.
 */
export function initViewportTracking() {
  if (typeof window === "undefined") return;
  const root = document.documentElement;
  const vv = window.visualViewport;

  const sync = () => {
    const h = vv?.height ?? window.innerHeight;
    const w = vv?.width ?? window.innerWidth;
    const inset = Math.max(0, window.innerHeight - h);
    root.style.setProperty("--app-vh", `${h}px`);
    root.style.setProperty("--app-vw", `${w}px`);
    root.style.setProperty("--kb-inset", `${inset}px`);
  };

  sync();

  if (vv) {
    // `resize` covers keyboard show/hide; `scroll` covers iOS pinch-zoom
    // and the case where the visual viewport shifts without resizing
    // (e.g. focused input scrolled within the layout viewport).
    vv.addEventListener("resize", sync);
    vv.addEventListener("scroll", sync);
  } else {
    window.addEventListener("resize", sync);
  }
}
