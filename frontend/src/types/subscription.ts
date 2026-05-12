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
  amount: number;
  currency: string; // ISO 4217: USD, RUB, EUR, GBP, KZT
  period: BillingPeriod;
  next_payment_at: string; // ISO date
  is_trial: boolean;
  trial_ends_at: string | null;
  is_auto_pay: boolean;
  logoUrl?: string;
}

export interface PartnerOffer {
  id: string;
  name: string;
  brand: BrandKey;
  tagline: string;
  cta_url: string;
  reward: string;
}

export interface UserSettings {
  isAdmin: boolean;
  isSubscribed: boolean;
  locale: "en" | "ru";
  defaultCurrency: string;
  cpaActive: boolean;
}
