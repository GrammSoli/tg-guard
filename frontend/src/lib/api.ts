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

// Coalesce concurrent 401-triggered reload attempts. The first failed request
// sets the flag, schedules a reload, and every subsequent in-flight request
// throws without scheduling its own reload.
let sessionExpiredHandled = false;

function handleSessionExpired() {
  if (sessionExpiredHandled) return;
  sessionExpiredHandled = true;
  // Fire-and-forget: avoid awaiting the dynamic import in the hot path.
  import("sonner").then(({ toast }) => {
    toast.error("Session expired — reopening…");
  }).catch(() => { /* noop */ });
  if (typeof window !== "undefined") {
    setTimeout(() => window.location.reload(), 1500);
  }
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
