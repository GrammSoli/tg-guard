/**
 * Brand identifier — the lookup key for POPULAR_SERVICES and the value
 * stored in DB on each Subscription / RoomService. Plain string rather
 * than a strict union because:
 *
 *   - the catalog now carries ~200 entries; keeping a TS union in
 *     lockstep with that table is mostly busywork
 *   - the backend treats `brand` as an opaque string; a stricter
 *     frontend type would create a false sense of validation
 *   - custom services land with "default" and can grow into arbitrary
 *     user-typed brands in the future
 *
 * Behaviour on unknown brand: ServiceLogo falls back to a colour-tinted
 * first-letter placeholder. Always rendered safely.
 */
export type BrandKey = string;

export type BillingPeriod = "monthly" | "yearly" | "weekly";

export interface Subscription {
  id: string;
  name: string;
  brand: BrandKey;
  tag?: string;
  /** Freeform user-supplied note to distinguish duplicates ("for family",
   *  "work card"). Shown muted next to the subscription name on the card. */
  note?: string;
  amount: number;
  currency: string; // ISO 4217: USD, RUB, EUR, GBP, KZT
  period: BillingPeriod;
  next_payment_at: string; // ISO date
  is_trial: boolean;
  trial_ends_at: string | null;
  is_auto_pay: boolean;
  logoUrl?: string;
  /** Custom-subscription appearance. Only honoured when `brand === "default"`.
   *  Name is one of the allow-listed lucide icons in `lib/customIcons.ts`.
   *  Colour is an id from `COLOR_LIST` (e.g. "blue", "emerald"). */
  icon_name?: string;
  icon_color?: string;
}

export interface PartnerOffer {
  id: string;
  name: string;
  brand: BrandKey;
  tagline: string;
  cta_url: string;
  reward: string;
}

/** Sponsored offer from admin panel, served by GET /api/v1/recommendations. */
export interface SponsoredOffer {
  id: number;
  title: string;
  description: string;
  badge_text: string;
  url: string;
  icon_name: string;
  target_language: "ru" | "en" | "all";
  is_active: boolean;
}

export interface UserSettings {
  isAdmin: boolean;
  isSubscribed: boolean;
  /** ISO timestamp when a time-limited Premium grant lapses. null =
   *  lifetime (or no Premium — disambiguate via isSubscribed). */
  premiumExpiresAt: string | null;
  locale: "en" | "ru";
  defaultCurrency: string;
  cpaActive: boolean;
  notificationsEnabled: boolean;
  /** IANA timezone name, e.g. "Europe/Moscow". Hydrated from the user's
   *  browser via Intl.DateTimeFormat on first sheet open. */
  timezone: string;
  /** Preferred notification time of day in "HH:MM" 24h format. */
  notificationTime: string;
}
