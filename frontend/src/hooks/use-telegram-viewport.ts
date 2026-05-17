import { useEffect, useState } from "react";
import {
  getTelegramViewportHeight,
  onTelegramViewportChanged,
} from "@/lib/telegram";

/**
 * Returns the live Telegram WebApp viewport height in pixels. Re-renders
 * whenever the viewport changes — which on iOS happens every time the
 * on-screen keyboard shows or hides.
 *
 * Use this for any sheet / drawer / modal that contains a text input,
 * instead of CSS `Nvh` / `Ndvh`. On iOS Telegram WebView, `100vh` and
 * (less reliably) `100dvh` measure the full window without accounting
 * for the keyboard — so a sheet sized as `max-h-[55vh]` keeps its
 * pre-keyboard height, leaving the user staring at a black void
 * between the form and the keyboard.
 *
 * Outside Telegram the hook falls back to window.innerHeight and a
 * resize listener, so dev-server use still gets sensible reflows.
 */
export function useTelegramViewportHeight(): number {
  const [h, setH] = useState(() => getTelegramViewportHeight());

  useEffect(() => {
    const sync = () => setH(getTelegramViewportHeight());

    // Telegram WebApp event. No-ops outside TMA.
    const offTg = onTelegramViewportChanged(sync);

    // Browser fallback — also fires inside TMA on orientation change,
    // which Telegram doesn't always re-broadcast.
    if (typeof window !== "undefined") {
      window.addEventListener("resize", sync);
      window.addEventListener("orientationchange", sync);
    }

    // Initial reconcile in case the value at first paint was stale.
    sync();

    return () => {
      offTg();
      if (typeof window !== "undefined") {
        window.removeEventListener("resize", sync);
        window.removeEventListener("orientationchange", sync);
      }
    };
  }, []);

  return h;
}
