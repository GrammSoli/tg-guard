import { create } from "zustand";
import { api } from "@/lib/api";

interface PaywallConfig {
  paywall_enabled: boolean;
  free_subs_limit: number;
  free_room_limit: number;
  // Plan-split pricing (Month / Lifetime). Stars are whole Telegram
  // Stars, locale-split; crypto is a single whole-USD amount per plan.
  // PremiumSheet picks the Stars pair by i18n language.
  price_stars_month_ru: number;
  price_stars_lifetime_ru: number;
  price_stars_month_en: number;
  price_stars_lifetime_en: number;
  price_crypto_month_usd_ru: number;
  price_crypto_lifetime_usd_ru: number;
  price_crypto_month_usd_en: number;
  price_crypto_lifetime_usd_en: number;
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
    price_stars_month_ru: 75,
    price_stars_lifetime_ru: 500,
    price_stars_month_en: 150,
    price_stars_lifetime_en: 1000,
    price_crypto_month_usd_ru: 1,
    price_crypto_lifetime_usd_ru: 10,
    price_crypto_month_usd_en: 2,
    price_crypto_lifetime_usd_en: 20,
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
