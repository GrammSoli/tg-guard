import { create } from "zustand";
import { api } from "@/lib/api";

interface PaywallConfig {
  paywall_enabled: boolean;
  free_subs_limit: number;
  free_room_limit: number;
  // Premium pricing, locale-split. Stars are whole Telegram Stars;
  // crypto values are whole USD. PremiumSheet picks the pair matching
  // the app's current i18n language.
  price_stars_ru: number;
  price_stars_en: number;
  price_crypto_usd_ru: number;
  price_crypto_usd_en: number;
}

interface PaywallStore {
  config: PaywallConfig;
  loaded: boolean;
  fetchConfig: () => Promise<void>;
}

export const usePaywallStore = create<PaywallStore>((set) => ({
  config: {
    paywall_enabled: false,
    free_subs_limit: 6,
    free_room_limit: 1,
    // Defaults mirror the backend AppSettings column defaults so the
    // PremiumSheet shows a sane price even before /config resolves.
    price_stars_ru: 50,
    price_stars_en: 100,
    price_crypto_usd_ru: 1,
    price_crypto_usd_en: 2,
  },
  loaded: false,

  fetchConfig: async () => {
    try {
      const data = await api<PaywallConfig>("/config");
      set({ config: data, loaded: true });
    } catch {
      // Fail silently — paywall stays disabled (permissive default).
      set({ loaded: true });
    }
  },
}));
