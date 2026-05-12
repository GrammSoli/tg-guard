import type { Subscription } from "@/types/subscription";

/**
 * Converts a subscription's amount to its monthly equivalent.
 * Weekly → ×4.345, Yearly → ÷12, Monthly → as-is.
 */
export const periodToMonthly = (s: Subscription): number =>
  s.period === "yearly" ? s.amount / 12 : s.period === "weekly" ? s.amount * 4.345 : s.amount;
