/**
 * HTTP client for SubGuard API.
 * Attaches Telegram initData to every request for authentication.
 */

import type { ZodType } from "zod";

const BASE = import.meta.env.VITE_API_URL ?? "/api/v1";

function getInitData(): string {
  try {
    return (window as any).Telegram?.WebApp?.initData ?? "";
  } catch {
    return "";
  }
}

interface ApiOptions extends Omit<RequestInit, "body" | "signal"> {
  body?: unknown;
  /**
   * Optional zod schema for runtime validation of the JSON response. When set,
   * a shape mismatch throws ApiError(0) and surfaces a console warning — far
   * easier to diagnose than a downstream undefined-access crash.
   */
  schema?: ZodType<unknown>;
  /**
   * AbortSignal for cancelling in-flight requests. Use when the caller (e.g.
   * a Zustand store opened by a soon-to-unmount sheet) should drop the
   * response if the user navigates away before it lands — audit O4. The api
   * helper translates the DOMException("AbortError") into a sentinel
   * ApiError(0, "aborted") so consumer try/catch branches can ignore it
   * without a name-string check.
   */
  signal?: AbortSignal;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// Coalesce concurrent 401 toasts. We deliberately do NOT auto-reload: an
// auto-reload on a persistently-401 backend produces an infinite reload
// loop inside the Telegram WebView. The user can close + reopen the bot
// manually; surfacing one clear toast is enough.
let sessionExpiredHandled = false;

// Global flag set when the server returns 403 account_banned.
// Uses Zustand so React components reactively re-render.
import { create } from "zustand";

interface BanStore {
  banned: boolean;
  setBanned: (v: boolean) => void;
}

export const useBanStore = create<BanStore>((set) => ({
  banned: false,
  setBanned: (v) => set({ banned: v }),
}));

/** @deprecated Use useBanStore().banned instead for reactive checks */
export function isBanned(): boolean { return useBanStore.getState().banned; }

// Global flag set when the server returns 503 maintenance_mode. Defined
// here (not in a Zustand store file) for the same reason as useBanStore:
// settingsStore imports this module, so a store file importing back into
// api.ts would be a circular dependency.
interface MaintenanceStore {
  maintenance: boolean;
  setMaintenance: (v: boolean) => void;
}

export const useMaintenanceStore = create<MaintenanceStore>((set) => ({
  maintenance: false,
  setMaintenance: (v) => set({ maintenance: v }),
}));

function handleSessionExpired() {
  if (sessionExpiredHandled) return;
  sessionExpiredHandled = true;
  import("sonner").then(({ toast }) => {
    toast.error("Session expired — please reopen the bot");
  }).catch(() => { /* noop */ });
}

/**
 * Performs an authenticated API request.
 * Automatically serializes body as JSON and attaches Telegram initData.
 */
export async function api<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const { body, headers: extraHeaders, schema, signal, ...rest } = options;

  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, {
      ...rest,
      headers: {
        "Content-Type": "application/json",
        "X-Telegram-Init-Data": getInitData(),
        ...extraHeaders,
      },
      body: body ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (err) {
    // Translate aborts so callers can `instanceof ApiError` instead of
    // sniffing for the platform-specific AbortError name.
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new ApiError(0, "aborted");
    }
    throw err;
  }

  if (res.status === 401) {
    // initData has expired (5-minute server window) or auth failed entirely.
    // Trigger a one-shot reload to pull a fresh initData from Telegram.
    handleSessionExpired();
    throw new ApiError(401, "session expired");
  }

  if (res.status === 403) {
    const body = await res.text().catch(() => "");
    if (body.includes("account_banned")) {
      useBanStore.getState().setBanned(true);
      throw new ApiError(403, "account_banned");
    }
    throw new ApiError(403, body || "Forbidden");
  }

  if (res.status === 503) {
    // Backend maintenance kill-switch. Flip the global flag so the root
    // component swaps the whole app for MaintenanceScreen. Other 503s
    // (proxy/LB hiccups) fall through to the generic !res.ok branch.
    const body = await res.text().catch(() => "");
    if (body.includes("maintenance_mode")) {
      useMaintenanceStore.getState().setMaintenance(true);
      throw new ApiError(503, "maintenance_mode");
    }
    throw new ApiError(503, body || "Service Unavailable");
  }

  if (!res.ok) {
    const text = await res.text().catch(() => "Unknown error");
    const apiErr = new ApiError(res.status, text);
    // 5xx = server-side failure the user can't fix and we need to know
    // about. Ship it to Sentry tagged with the endpoint. 4xx is client
    // error (validation, etc.) — left out to avoid noise. Sentry is a
    // no-op when VITE_SENTRY_DSN is unset.
    if (res.status >= 500) {
      void import("@sentry/react").then((Sentry) => {
        Sentry.captureException(apiErr, {
          tags: { "api.path": path, "api.method": (rest.method as string) ?? "GET" },
          extra: { status: res.status, body: text.slice(0, 500) },
        });
      });
    }
    throw apiErr;
  }

  // Handle empty responses (204 No Content)
  const contentType = res.headers.get("content-type");
  if (!contentType || !contentType.includes("application/json")) {
    return {} as T;
  }

  const data = await res.json();
  if (schema) {
    const result = schema.safeParse(data);
    if (!result.success) {
      console.error(`[api] ${path} response shape mismatch`, result.error.issues);
      throw new ApiError(0, "invalid response shape");
    }
    return result.data as T;
  }
  return data as T;
}
