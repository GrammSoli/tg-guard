import { create } from "zustand";
import { api } from "@/lib/api";

interface PaywallConfig {
  paywall_enabled: boolean;
  free_subs_limit: number;
  free_room_limit: number;
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
