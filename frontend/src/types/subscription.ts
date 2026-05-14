export type BrandKey =
  | "netflix"
  | "spotify"
  | "youtube"
  | "icloud"
  | "apple"
  | "applemusic"
  | "telegram"
  | "disney"
  | "notion"
  | "chatgpt"
  | "midjourney"
  | "figma"
  | "canva"
  | "dropbox"
  | "googleone"
  | "xbox"
  | "playstation"
  | "twitch"
  | "hbomax"
  | "crunchyroll"
  | "nordvpn"
  | "expressvpn"
  | "onepassword"
  | "todoist"
  | "linear"
  | "slack"
  | "zoom"
  | "duolingo"
  | "strava"
  | "headspace"
  | "github"
  | "adobe"
  | "yandexplus"
  | "vkmusic"
  | "mts"
  | "megogo"
  | "kinopoisk"
  | "default";

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
