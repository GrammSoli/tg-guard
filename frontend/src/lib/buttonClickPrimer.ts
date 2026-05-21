/**
 * Mirror of lib/inputFocusPrimer, but for <button> elements.
 *
 * iOS Telegram WebView drops the synthetic `click` on the first tap
 * inside a freshly mounted drawer — same root cause that needed the
 * pointerdown→focus shortcut for inputs (PR #113). Without click,
 * onClick never fires and the button looks dead until the user has
 * tapped it 2-3 times. PR #116 (vaul handleOnly) eliminated one
 * contributing factor but didn't cover the platform-level click drop.
 *
 * Approach: globally observe pointerdown / pointerup on document. If
 * the user tapped a button (no significant movement, short duration)
 * and iOS does NOT deliver a `click` within a brief window, dispatch
 * a synthetic click on the button ourselves so its onClick runs.
 * When iOS DOES deliver click normally, we do nothing — the small
 * timing window de-duplicates against the native event.
 *
 * Side effects:
 *   - Safe for normal lambdas (`onClick={() => setStep("…")}`); they
 *     don't read the event object.
 *   - Disabled buttons are a no-op via .click() — browsers ignore.
 *   - Keyboard a11y (Enter / Space) still fires native click as usual.
 *
 * Call once at startup from main.tsx.
 */
const TAP_MOVE_TOLERANCE_PX = 10;
const TAP_MAX_DURATION_MS = 500;
const NATIVE_CLICK_WAIT_MS = 60;
const DEDUPE_WINDOW_MS = 120;

interface PressState {
  button: HTMLButtonElement;
  x: number;
  y: number;
  time: number;
}

export function initButtonClickPrimer() {
  if (typeof document === "undefined") return;

  let press: PressState | null = null;
  let lastClickAt = 0;

  // Capture-phase listener so we see the click even if a stopPropagation
  // handler runs later in the bubble phase.
  document.addEventListener(
    "click",
    () => {
      lastClickAt = performance.now();
    },
    true,
  );

  document.addEventListener(
    "pointerdown",
    (e) => {
      const target = e.target;
      if (!(target instanceof Element)) return;
      const button = target.closest("button");
      if (!button || button.disabled) return;
      press = {
        button,
        x: e.clientX,
        y: e.clientY,
        time: performance.now(),
      };
    },
    true,
  );

  document.addEventListener(
    "pointerup",
    (e) => {
      const start = press;
      press = null;
      if (!start) return;

      // Reject anything that doesn't look like a quick stationary tap —
      // a drag, a long-press, or a finger that wandered off the button.
      const dx = Math.abs(e.clientX - start.x);
      const dy = Math.abs(e.clientY - start.y);
      const dt = performance.now() - start.time;
      if (
        dx > TAP_MOVE_TOLERANCE_PX ||
        dy > TAP_MOVE_TOLERANCE_PX ||
        dt > TAP_MAX_DURATION_MS
      ) {
        return;
      }

      const button = start.button;

      // Wait briefly for the platform-native click. If it arrives we're
      // done — the dedupe check below sees a recent click and bails out.
      window.setTimeout(() => {
        if (performance.now() - lastClickAt < DEDUPE_WINDOW_MS) return;
        if (button.isConnected && !button.disabled) {
          button.click();
        }
      }, NATIVE_CLICK_WAIT_MS);
    },
    true,
  );

  // Cancel a pending press if the gesture turns into a scroll / is taken
  // over by another element (e.g. vaul's drag handle picks it up).
  document.addEventListener(
    "pointercancel",
    () => {
      press = null;
    },
    true,
  );
}
