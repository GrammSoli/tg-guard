import { create } from "zustand";
import { api } from "@/lib/api";

/**
 * Live exchange rates fetched from /api/v1/fx (which forwards what the
 * backend currency worker caches in Redis daily). All rates are
 * USD-denominated: rates[X] = "how many X you get for 1 USD".
 *
 * convertCurrency() in lib/currencyRates.ts cross-converts via USD:
 *   amount_to = (amount_from / rates[from]) * rates[to]
 *
 * On cache miss / network failure the store stays empty and
 * convertCurrency falls back to a hard-coded 2025-mid snapshot — better
 * for the user to see "approximate" numbers than NaN.
 */
interface FxRatesResponse {
  base: string;
  rates: Record<string, number>;
}

interface FxStore {
  /** USD-base rates. Empty until first successful fetchRates(). */
  rates: Record<string, number>;
  /** True after the first fetch resolves (success or fail) so the UI
   *  knows the static fallback is the FINAL answer, not a placeholder. */
  loaded: boolean;
  fetchRates: () => Promise<void>;
}

export const useFxStore = create<FxStore>((set) => ({
  rates: {},
  loaded: false,

  fetchRates: async () => {
    try {
      const data = await api<FxRatesResponse>("/fx");
      // Defensive: backend serves {} on cache miss. Keep the previous
      // rates map in that case so a transient Redis outage doesn't
      // wipe out a working snapshot.
      if (data.rates && Object.keys(data.rates).length > 0) {
        set({ rates: data.rates, loaded: true });
      } else {
        set({ loaded: true });
      }
    } catch {
      // Silent — convertCurrency falls back to static table.
      set({ loaded: true });
    }
  },
}));
