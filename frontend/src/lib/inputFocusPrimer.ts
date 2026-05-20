/**
 * Force focus on inputs via `pointerdown` instead of relying on the
 * synthetic `click` → focus chain.
 *
 * Telegram WebView on iOS drops the synthetic click on the first
 * tap of an input inside a freshly mounted drawer (PR #111 logs
 * captured this directly: pointerdown + touchstart arrive, no
 * mousedown / focusin / click follow). The user has to tap 2-3
 * times before the keyboard finally appears. PR #112 reduced but
 * did not eliminate the cold-start case.
 *
 * `pointerdown` IS delivered reliably and fires inside the user-
 * gesture window, where iOS permits `.focus()` to open the on-screen
 * keyboard. By focusing the input on pointerdown ourselves we
 * bypass the broken click chain entirely.
 *
 * Safe on Android / desktop: focusing slightly earlier than the
 * native flow is a no-op when click also fires (second focus on the
 * same element does nothing). Only input/textarea elements are
 * targeted — buttons, links, selects (iOS picker) are left alone.
 *
 * Call once at startup from main.tsx.
 */
export function initInputFocusPrimer() {
  if (typeof document === "undefined") return;
  document.addEventListener(
    "pointerdown",
    (e) => {
      const t = e.target;
      if (
        !(t instanceof HTMLInputElement) &&
        !(t instanceof HTMLTextAreaElement)
      ) {
        return;
      }
      // Don't fight disabled / readonly fields or hidden helpers.
      if (t.disabled || t.readOnly || t.type === "hidden") return;
      if (document.activeElement === t) return;
      try {
        t.focus({ preventScroll: false });
      } catch {
        /* noop — focus() can throw if the element became unmounted
           between dispatch and handler. */
      }
    },
    true,
  );
}
