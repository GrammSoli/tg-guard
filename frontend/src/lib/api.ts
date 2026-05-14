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

interface ApiOptions extends Omit<RequestInit, "body"> {
  body?: unknown;
  /**
   * Optional zod schema for runtime validation of the JSON response. When set,
   * a shape mismatch throws ApiError(0) and surfaces a console warning — far
   * easier to diagnose than a downstream undefined-access crash.
   */
  schema?: ZodType<unknown>;
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
  const { body, headers: extraHeaders, schema, ...rest } = options;

  const res = await fetch(`${BASE}${path}`, {
    ...rest,
    headers: {
      "Content-Type": "application/json",
      "X-Telegram-Init-Data": getInitData(),
      ...extraHeaders,
    },
    body: body ? JSON.stringify(body) : undefined,
  });

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

  if (!res.ok) {
    const text = await res.text().catch(() => "Unknown error");
    throw new ApiError(res.status, text);
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
