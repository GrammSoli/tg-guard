import { create } from "zustand";
import type { UserSettings } from "@/types/subscription";
import { api } from "@/lib/api";

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
  is_admin: boolean;
}

interface SettingsStore {
  settings: UserSettings;
  user: { name: string; username: string; photoUrl: string } | null;
  loading: boolean;
  fetchProfile: () => Promise<void>;
  updateSettings: (patch: Partial<UserSettings>) => void;
}

const defaultSettings: UserSettings = {
  locale: "en",
  defaultCurrency: "USD",
  isAdmin: false,
  isSubscribed: false,
  cpaActive: false,
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

      set({
        settings: {
          locale,
          defaultCurrency: me.base_currency ?? "USD",
          isAdmin: me.is_admin ?? false,
          isSubscribed: me.is_donator ?? false,
          cpaActive: false,
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

  updateSettings: (patch) => {
    // Synchronous state update only — side effects must live outside set().
    // React 18 strict-mode can execute reducers twice; running fetch/toast
    // inside would fire twice and create undefined state on rollback.
    const prev = get().settings;
    const next = { ...prev, ...patch };
    set({ settings: next });

    const apiPatch: Record<string, string> = {};
    if (patch.locale) apiPatch.locale = patch.locale;
    if (patch.defaultCurrency) apiPatch.base_currency = patch.defaultCurrency;
    if (Object.keys(apiPatch).length === 0) return;

    api("/me", { method: "PATCH", body: apiPatch }).catch(async () => {
      set({ settings: prev });
      const { toast } = await import("sonner");
      toast.error("Failed to save settings");
    });
  },
}));
