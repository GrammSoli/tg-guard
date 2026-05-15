import { useFxStore } from "@/stores/fxStore";

/**
 * Hard-coded fallback rates — used only when the live fxStore is empty
 * (first frame before fetchRates resolves, or persistent Redis outage).
 * Semantics match the backend: rates[X] = "how many X you get for 1
 * USD". Approximate, mid-2025.
 *
 * Keep `STATIC_USD_RATES` and the active currencies list in sync — both
 * are reciprocal versions of the legacy `TO_USD` table this file used
 * before audit #16.
 */
const STATIC_USD_RATES: Record<string, number> = {
  USD: 1,
  EUR: 0.926,
  GBP: 0.787,
  RUB: 90.9,
  KZT: 500,
};

const SUPPORTED_CURRENCIES = ["USD", "EUR", "RUB", "GBP", "KZT"] as const;
export type SupportedCurrency = (typeof SUPPORTED_CURRENCIES)[number];
export { SUPPORTED_CURRENCIES };

/**
 * `useFxRates` is the React-reactive accessor. Components doing currency
 * conversion subscribe via this hook (or include the returned `rates`
 * object in a useMemo dependency list) so they re-render when fresh
 * rates land from /api/v1/fx.
 *
 * Returns the live USD-base rates with the static table merged
 * underneath — so any pair the live feed doesn't cover (e.g. a brand
 * new currency added to STATIC but not yet to the worker) still
 * resolves. Live values always win when both exist.
 */
export function useFxRates(): Record<string, number> {
  const live = useFxStore((s) => s.rates);
  return { ...STATIC_USD_RATES, ...live };
}

/**
 * Convert `amount` from `from` currency to `to` currency.
 *
 * NOT a React hook — reads the current store snapshot via getState().
 * Components that want their conversions to re-render automatically
 * when rates land should subscribe via useFxRates() (or include
 * useFxStore((s) => s.rates) in their useMemo deps).
 *
 * Falls back through STATIC_USD_RATES for any currency the live feed
 * doesn't cover. Unknown currencies (not in either table) degrade to
 * 1:1 — same as the previous behaviour.
 */
export function convertCurrency(
  amount: number,
  from: string,
  to: string,
): number {
  if (from === to) return amount;
  if (!Number.isFinite(amount)) return 0;
  const live = useFxStore.getState().rates;
  const fromRate = live[from] ?? STATIC_USD_RATES[from] ?? 1;
  const toRate = live[to] ?? STATIC_USD_RATES[to] ?? 1;
  if (fromRate === 0) return 0;
  return (amount / fromRate) * toRate;
}

/** Symbol for quick display */
export function currencySymbol(code: string): string {
  const map: Record<string, string> = {
    USD: "$",
    EUR: "€",
    GBP: "£",
    RUB: "₽",
    KZT: "₸",
  };
  return map[code] ?? code;
}
