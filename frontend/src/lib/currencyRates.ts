/**
 * Static cross-rates to USD (will be replaced by API-fetched rates with backend).
 * Rates approximate as of mid-2025.
 */
const TO_USD: Record<string, number> = {
  USD: 1,
  EUR: 1.08,
  GBP: 1.27,
  RUB: 0.011,
  KZT: 0.002,
};

const SUPPORTED_CURRENCIES = ["USD", "EUR", "RUB", "GBP", "KZT"] as const;
export type SupportedCurrency = (typeof SUPPORTED_CURRENCIES)[number];
export { SUPPORTED_CURRENCIES };

/**
 * Convert `amount` from `from` currency to `to` currency.
 * Unknown currencies fall back to 1:1 with USD.
 */
export function convertCurrency(
  amount: number,
  from: string,
  to: string,
): number {
  if (from === to) return amount;
  if (!Number.isFinite(amount)) return 0;
  const fromRate = TO_USD[from] ?? 1;
  const toRate = TO_USD[to] ?? 1;
  if (toRate === 0) return 0;
  return (amount * fromRate) / toRate;
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
