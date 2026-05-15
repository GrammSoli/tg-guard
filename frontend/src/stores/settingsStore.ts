import { create } from "zustand";
import * as Sentry from "@sentry/react";
import type { UserSettings } from "@/types/subscription";
import { api, ApiError } from "@/lib/api";

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

      set({
        settings: {
          locale,
          defaultCurrency: me.base_currency ?? "USD",
          isAdmin: me.is_admin ?? false,
          isSubscribed: me.is_donator ?? false,
          premiumExpiresAt: me.premium_expires_at ?? null,
          cpaActive: false,
          notificationsEnabled: me.notifications_enabled ?? true,
          timezone: me.timezone ?? "UTC",
          notificationTime: me.notification_time ?? "10:00",
        },
        user: {
          name: [me.first_name, me.last_name].filter(Boolean).join(" "),
          username: me.username ?? "",
          photoUrl: me.photo_url ?? "",
        },
        loading: false,
      });
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
