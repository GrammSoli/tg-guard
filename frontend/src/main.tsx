import React from "react";
import ReactDOM from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import * as Sentry from "@sentry/react";
import telegramAnalytics from "@telegram-apps/analytics";
import { getRouter } from "./router";
import { expandMiniApp, isLaunchedFromTelegram, tgReady } from "@/lib/telegram";
import { initViewportTracking } from "@/lib/viewport";
import { FocusDebugPanel } from "@/components/dev/FocusDebugPanel";
import "./styles.css";
import "@/lib/i18n";

// Tell Telegram the WebApp is alive (hides the platform spinner) and
// expand the WebView to full height. Done before mount so the
// viewportChanged event from expand() is dispatched against a viewport
// that already has its post-expand dimensions when components subscribe.
tgReady();
expandMiniApp();

// Wire the W3C VisualViewport API to CSS custom properties (--app-vh,
// --kb-inset). Bottom sheets and any other keyboard-aware UI consume
// these directly from CSS — no React state, no platform guessing.
initViewportTracking();

// ── Sentry ───────────────────────────────────────────────
// No-op when VITE_SENTRY_DSN is unset (local dev needs no account).
// @sentry/react auto-installs window 'error' + 'unhandledrejection'
// handlers on init, so uncaught errors and rejected promises ship
// without extra wiring. RootErrorBoundary below additionally reports
// React render errors with the component stack.
const SENTRY_DSN = import.meta.env.VITE_SENTRY_DSN as string | undefined;
if (SENTRY_DSN) {
  Sentry.init({
    dsn: SENTRY_DSN,
    environment: import.meta.env.MODE,
    release: import.meta.env.VITE_APP_VERSION as string | undefined,
    // Errors only — no perf tracing, keeps event volume + quota low.
    tracesSampleRate: 0,
    // Aborted fetches (user navigated away) and bare promise rejections
    // are expected noise in a Telegram WebView — drop them.
    ignoreErrors: ["AbortError", "Non-Error promise rejection captured"],
    // Session Replay: record DOM only on errors. maskAllText + blockAllMedia
    // because this is a billing UI showing amounts, payment providers and
    // Telegram usernames — default-mask everything, opt back in per-element
    // later if needed.
    integrations: [
      Sentry.replayIntegration({
        maskAllText: true,
        blockAllMedia: true,
      }),
    ],
    replaysSessionSampleRate: 0,
    replaysOnErrorSampleRate: 1.0,
  });
}

// ── Telegram Analytics ───────────────────────────────────
// No-op when the env vars are unset. Tracks app launches and
// TON Connect events automatically. The token and app identifier
// are issued by @DataChief_bot. Init must run before render.
//
// Gated on isLaunchedFromTelegram(): outside Telegram the SDK can't
// retrieve launch params and throws LaunchParamsRetrieveError as an
// unhandled rejection — noise for anyone (or any crawler) hitting
// the domain in a plain browser.
const TGA_TOKEN = import.meta.env.VITE_TGA_TOKEN as string | undefined;
const TGA_APP_NAME = import.meta.env.VITE_TGA_APP_NAME as string | undefined;
if (TGA_TOKEN && TGA_APP_NAME && isLaunchedFromTelegram()) {
  telegramAnalytics.init({ token: TGA_TOKEN, appName: TGA_APP_NAME });
}

const router = getRouter();

// Top-level error boundary. TanStack Router has per-route errorComponent
// but a render error inside e.g. SharedRoomSheet escapes that and would
// otherwise leave the user with a blank screen inside Telegram, where
// devtools are unavailable.
interface BoundaryState {
  error: Error | null;
}

class RootErrorBoundary extends React.Component<
  { children: React.ReactNode },
  BoundaryState
> {
  state: BoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): BoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("[root-error-boundary]", error, info);
    // Ship the render error with its component stack so Sentry shows
    // which subtree threw, not just the JS stack. No-op without a DSN.
    Sentry.captureException(error, {
      contexts: {
        react: { componentStack: info.componentStack },
      },
      tags: { boundary: "root" },
    });
  }

  reset = () => {
    this.setState({ error: null });
  };

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <div className="flex min-h-screen items-center justify-center bg-background px-4">
        <div className="max-w-md text-center">
          <h1 className="text-xl font-semibold tracking-tight text-foreground">
            Something went wrong
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {this.state.error.message || "Unexpected error"}
          </p>
          <div className="mt-6 flex flex-wrap justify-center gap-2">
            <button
              onClick={this.reset}
              className="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
            >
              Try again
            </button>
            <button
              onClick={() => window.location.reload()}
              className="inline-flex items-center justify-center rounded-md border border-input bg-background px-4 py-2 text-sm font-medium text-foreground transition-colors hover:bg-accent"
            >
              Reload
            </button>
          </div>
        </div>
      </div>
    );
  }
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <RootErrorBoundary>
      <RouterProvider router={router} />
      <FocusDebugPanel />
    </RootErrorBoundary>
  </React.StrictMode>,
);
