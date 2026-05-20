import { useEffect, useState } from "react";

/**
 * On-screen debug panel for diagnosing first-tap focus issues inside
 * Telegram WebView (especially iOS). Logs the order in which touch /
 * pointer / focus / click events fire on document capture, so the bug
 * report becomes "tap landed but focusin never followed" or "focusin
 * fired and was immediately stolen by focusout" rather than "input
 * doesn't work".
 *
 * Enable by launching the Mini App with start_param=focus-debug, e.g.
 *   https://t.me/<bot>/<short>?startapp=focus-debug
 * or by adding ?focus-debug / #focus-debug to the URL. Regular users
 * never see the panel.
 *
 * The panel sits above the keyboard via `bottom: var(--kb-inset)` and
 * has `pointer-events: none` so it cannot intercept the taps it is
 * trying to observe.
 */
export function FocusDebugPanel() {
  const [enabled, setEnabled] = useState(false);
  const [logs, setLogs] = useState<string[]>([]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const tg = (
      window as unknown as { Telegram?: { WebApp?: { initDataUnsafe?: { start_param?: string } } } }
    ).Telegram?.WebApp;
    const startParam = tg?.initDataUnsafe?.start_param;
    const hashFlag = window.location.hash.includes("focus-debug");
    const queryFlag = new URLSearchParams(window.location.search).has("focus-debug");
    const isEnabled = startParam === "focus-debug" || hashFlag || queryFlag;
    setEnabled(isEnabled);
    if (!isEnabled) return;

    const describeTarget = (target: EventTarget | null): string => {
      const el = target as HTMLElement | null;
      if (!el || !el.tagName) return "?";
      const tag = el.tagName.toLowerCase();
      const placeholder = (el as HTMLInputElement).placeholder;
      const label = placeholder?.slice(0, 18) ?? el.id ?? el.className?.slice(0, 18) ?? "";
      return `${tag}${label ? `[${label}]` : ""}`;
    };

    const push = (line: string) => {
      setLogs((curr) => [...curr.slice(-29), line]);
    };

    const handler = (e: Event) => {
      const t = ((performance.now() | 0) % 100000).toString().padStart(5, "0");
      push(`${t} ${e.type} ${describeTarget(e.target)}`);
    };

    const events = [
      "touchstart",
      "pointerdown",
      "mousedown",
      "click",
      "focusin",
      "focusout",
    ];
    events.forEach((ev) => document.addEventListener(ev, handler, true));
    return () => events.forEach((ev) => document.removeEventListener(ev, handler, true));
  }, []);

  if (!enabled) return null;

  return (
    <div
      style={{
        position: "fixed",
        bottom: "var(--kb-inset, 0px)",
        right: 0,
        zIndex: 99999,
        background: "rgba(0, 0, 0, 0.85)",
        color: "#0f0",
        padding: "4px 6px",
        fontSize: 9,
        lineHeight: 1.25,
        fontFamily: "monospace",
        maxWidth: "55%",
        maxHeight: "35vh",
        overflow: "auto",
        pointerEvents: "none",
        borderTopLeftRadius: 6,
      }}
    >
      {logs.length === 0 ? (
        <div>focus-debug ready, tap an input…</div>
      ) : (
        logs.map((l, i) => <div key={i}>{l}</div>)
      )}
    </div>
  );
}
