import { create } from "zustand";
import * as Sentry from "@sentry/react";
import type { UserSettings } from "@/types/subscription";
import { api, ApiError } from "@/lib/api";

/**
 * Read the user's IANA timezone from the browser. Returns "UTC" when the
 * Intl API is missing or throws — keeps callers branch-free.
 */
function detectBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

/**
 * Push the detected browser timezone to the backend with exponential
 * backoff on transient failures. Stops on a 4xx (the value won't get
 * any more valid on a retry) and gives up after the third attempt.
 *
 * Fire-and-forget by design — `fetchProfile` must not block on this.
 * Worst case we land back at UTC server-side and the next
 * NotificationsSheet open surfaces the mismatch.
 */
async function syncTimezoneWithBackoff(tz: string): Promise<void> {
  const delaysMs = [0, 1000, 4000];
  for (let attempt = 0; attempt < delaysMs.length; attempt++) {
    if (delaysMs[attempt] > 0) {
      await new Promise((r) => setTimeout(r, delaysMs[attempt]));
    }
    try {
      await api("/me", { method: "PATCH", body: { timezone: tz } });
      return;
    } catch (err) {
      // 4xx → server actively rejected (invalid IANA name, ban, etc.).
      // Retrying won't change the verdict.
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) {
        console.warn("[settings] tz sync rejected by server", err);
        return;
      }
      if (attempt === delaysMs.length - 1) {
        console.warn("[settings] tz sync gave up after retries", err);
        Sentry.captureException(err);
      }
    }
  }
}

interface MeResponse {
  id: number;
  telegram_id: number;
  first_name: string;
  last_name: string;
  username: string;
  photo_url: string;
  locale: "en" | "ru";
  timezone: string;
  base_currency: string;
  is_donator: boolean;
  premium_expires_at: string | null;
  is_admin: boolean;
  notifications_enabled: boolean;
  notification_time: string;
}

interface SettingsStore {
  settings: UserSettings;
  user: { name: string; username: string; photoUrl: string } | null;
  loading: boolean;
  fetchProfile: () => Promise<void>;
  updateSettings: (patch: Partial<UserSettings>) => Promise<void>;
}

const defaultSettings: UserSettings = {
  locale: "en",
  defaultCurrency: "USD",
  isAdmin: false,
  isSubscribed: false,
  premiumExpiresAt: null,
  cpaActive: false,
  notificationsEnabled: true,
  timezone: "UTC",
  notificationTime: "10:00",
};

export const useSettingsStore = create<SettingsStore>((set, get) => ({
  settings: defaultSettings,
  user: null,
  loading: true,

  fetchProfile: async () => {
    set({ loading: true });
    try {
      const me = await api<MeResponse>("/me");
      const locale = me.locale ?? "en";
      import("@/lib/i18n").then(({ default: i18n }) => {
        i18n.changeLanguage(locale);
      });

      // Attach identity to every Sentry event for the rest of the
      // session. Set once here on profile load — Sentry keeps the user
      // on the global scope, so a later 5xx or render crash is already
      // tagged with "who". No-op when VITE_SENTRY_DSN is unset. ID is
      // the internal user_id (stable); username + telegram_id make the
      // dashboard searchable by "@Ivanov".
      Sentry.setUser({
        id: String(me.id),
        username: me.username || undefined,
        telegram_id: me.telegram_id,
      });

      const serverTz = me.timezone ?? "UTC";
      set({
        settings: {
          locale,
          defaultCurrency: me.base_currency ?? "USD",
          isAdmin: me.is_admin ?? false,
          isSubscribed: me.is_donator ?? false,
          premiumExpiresAt: me.premium_expires_at ?? null,
          cpaActive: false,
          notificationsEnabled: me.notifications_enabled ?? true,
          timezone: serverTz,
          notificationTime: me.notification_time ?? "10:00",
        },
        user: {
          name: [me.first_name, me.last_name].filter(Boolean).join(" "),
          username: me.username ?? "",
          photoUrl: me.photo_url ?? "",
        },
        loading: false,
      });

      // Eager timezone sync: when the server has the bare default ("UTC"
      // or empty), assume this is the first auth from a fresh browser
      // and push the detected IANA name up. A user who already picked
      // a non-UTC zone (e.g. via NotificationsSheet from another device)
      // is NOT overwritten — we'd otherwise stamp on Asia/Tokyo every
      // time they open the app from a laptop in London.
      const browserTz = detectBrowserTimezone();
      if (browserTz && browserTz !== serverTz && (serverTz === "UTC" || serverTz === "")) {
        set((s) => ({ settings: { ...s.settings, timezone: browserTz } }));
        void syncTimezoneWithBackoff(browserTz);
      }
    } catch {
      set({ loading: false });
    }
  },

  /**
   * Patch user settings against the backend.
   *
   * - Sync state update first (no side-effects inside set; see audit O3).
   * - PATCH /me, await it, throw on failure so callers can show specific
   *   error UX. We DON'T silently swallow the error here — the previous
   *   "Failed to save settings" toast hid real backend errors. The caller
   *   sees an ApiError with the server's actual message.
   * - On error: roll the optimistic state back BEFORE rethrowing.
   */
  updateSettings: async (patch) => {
    const prev = get().settings;
    const next = { ...prev, ...patch };
    set({ settings: next });

    const apiPatch: Record<string, string | boolean> = {};
    if (patch.locale) apiPatch.locale = patch.locale;
    if (patch.defaultCurrency) apiPatch.base_currency = patch.defaultCurrency;
    if (patch.notificationsEnabled !== undefined) {
      apiPatch.notifications_enabled = patch.notificationsEnabled;
    }
    if (patch.timezone) apiPatch.timezone = patch.timezone;
    if (patch.notificationTime) apiPatch.notification_time = patch.notificationTime;

    if (Object.keys(apiPatch).length === 0) return;

    try {
      await api("/me", { method: "PATCH", body: apiPatch });
    } catch (err) {
      set({ settings: prev });
      // Ban errors are handled globally by useBanStore → BannedScreen.
      // Don't re-throw — callers would show a misleading toast.
      if (err instanceof ApiError && err.message === "account_banned") return;
      if (err instanceof ApiError) {
        throw err;
      }
      throw new ApiError(0, (err as Error)?.message ?? "network error");
    }
  },
}));
