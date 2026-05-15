import type { Subscription, UserSettings } from "@/types/subscription";

const today = new Date();
const inDays = (d: number) => {
  const t = new Date(today);
  t.setDate(t.getDate() + d);
  return t.toISOString();
};

export const mockSubscriptions: Subscription[] = [
  {
    id: "1",
    name: "Netflix",
    brand: "netflix",
    tag: "Entertainment",
    amount: 22.99,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(2),
    is_trial: false,
    trial_ends_at: null,
    is_auto_pay: true,
    logoUrl: "https://thesvg.org/icons/netflix/default.svg",
  },
  {
    id: "2",
    name: "Spotify",
    brand: "spotify",
    tag: "Music",
    amount: 16.99,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(6),
    is_trial: false,
    trial_ends_at: null,
    is_auto_pay: true,
    logoUrl: "https://thesvg.org/icons/spotify/default.svg",
  },
  {
    id: "3",
    name: "YouTube Premium",
    brand: "youtube",
    tag: "Entertainment",
    amount: 13.99,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(12),
    is_trial: true,
    trial_ends_at: inDays(12),
    is_auto_pay: true,
    logoUrl: "https://thesvg.org/icons/youtube/default.svg",
  },
  {
    id: "4",
    name: "iCloud+",
    brand: "icloud",
    tag: "Storage",
    amount: 2.99,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(18),
    is_trial: false,
    trial_ends_at: null,
    is_auto_pay: false,
    logoUrl: "https://thesvg.org/icons/icloud/default.svg",
  },
  {
    id: "5",
    name: "Notion",
    brand: "notion",
    tag: "Productivity",
    amount: 8.0,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(21),
    is_trial: false,
    trial_ends_at: null,
    is_auto_pay: true,
    logoUrl: "https://thesvg.org/icons/notion/default.svg",
  },
  {
    id: "6",
    name: "Disney+",
    brand: "disney",
    tag: "Entertainment",
    amount: 9.99,
    currency: "USD",
    period: "monthly",
    next_payment_at: inDays(28),
    is_trial: false,
    trial_ends_at: null,
    is_auto_pay: false,
    logoUrl: "https://thesvg.org/icons/disney/default.svg",
  },
];

export const mockUserSettings: UserSettings = {
  isAdmin: true,
  isSubscribed: true,
  premiumExpiresAt: null,
  locale: "en",
  defaultCurrency: "USD",
  cpaActive: true,
  notificationsEnabled: true,
  timezone: "UTC",
  notificationTime: "10:00",
};

export interface SharedRoom {
  id: string;
  name: string;
  members: number;
  total_per_member: number;
  currency: string;
  services: { brand: import("@/types/subscription").BrandKey; logoUrl: string }[];
}

export const mockSharedRooms: SharedRoom[] = [
  {
    id: "r1",
    name: "Family Office",
    members: 3,
    total_per_member: 12.5,
    currency: "USD",
    services: [
      { brand: "netflix", logoUrl: "https://thesvg.org/icons/apple/default.svg" },
      { brand: "disney", logoUrl: "https://thesvg.org/icons/disney/default.svg" },
      { brand: "spotify", logoUrl: "https://thesvg.org/icons/spotify/default.svg" },
    ],
  },
  {
    id: "r2",
    name: "Roommates",
    members: 4,
    total_per_member: 8.75,
    currency: "USD",
    services: [
      { brand: "youtube", logoUrl: "https://thesvg.org/icons/youtube/default.svg" },
      { brand: "icloud", logoUrl: "https://thesvg.org/icons/icloud/default.svg" },
    ],
  },
  {
    id: "r3",
    name: "Study Group",
    members: 5,
    total_per_member: 4.2,
    currency: "USD",
    services: [
      { brand: "notion", logoUrl: "https://thesvg.org/icons/notion/default.svg" },
      { brand: "applemusic", logoUrl: "https://thesvg.org/icons/applemusic/default.svg" },
      { brand: "spotify", logoUrl: "https://thesvg.org/icons/spotify/default.svg" },
    ],
  },
];

export type ServiceCategory =
  | "Entertainment"
  | "Music"
  | "Productivity"
  | "Social"
  | "Utilities"
  | "Health & Fitness"
  | "Finance"
  | "AI"
  | "Games"
  | "Cloud"
  | "VPN"
  | "Design";

export const SERVICE_CATEGORIES: ServiceCategory[] = [
  "Entertainment",
  "Music",
  "Productivity",
  "Social",
  "Utilities",
  "Health & Fitness",
  "Finance",
  "AI",
  "Games",
  "Cloud",
  "VPN",
  "Design",
];

export interface PopularService {
  id: string;
  name: string;
  brand: import("@/types/subscription").BrandKey;
  brandColor: string;
  defaultAmount: number;
  defaultCurrency: string;
  tag: string;
  category: ServiceCategory;
  logoUrl?: string;
}

export const POPULAR_SERVICES: PopularService[] = [
  // Entertainment
  { id: "netflix", name: "Netflix", brand: "netflix", brandColor: "#E50914", defaultAmount: 15.49, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/netflix/default.svg" },
  { id: "disney", name: "Disney+", brand: "disney", brandColor: "#0F47BA", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/disney/default.svg" },
  { id: "youtube", name: "YouTube Premium", brand: "youtube", brandColor: "#FF0000", defaultAmount: 13.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/youtube/default.svg" },
  { id: "hbomax", name: "HBO Max", brand: "hbomax", brandColor: "#002BE7", defaultAmount: 15.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/hbomax/default.svg" },
  { id: "crunchyroll", name: "Crunchyroll", brand: "crunchyroll", brandColor: "#F47521", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Anime", category: "Entertainment", logoUrl: "https://thesvg.org/icons/crunchyroll/default.svg" },
  { id: "twitch", name: "Twitch Turbo", brand: "twitch", brandColor: "#9146FF", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/twitch/default.svg" },
  { id: "kinopoisk", name: "Кинопоиск", brand: "kinopoisk", brandColor: "#FF5500", defaultAmount: 269, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/kinopoisk/default.svg" },
  { id: "megogo", name: "MEGOGO", brand: "megogo", brandColor: "#22B14C", defaultAmount: 199, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", logoUrl: "https://thesvg.org/icons/megogo/default.svg" },
  { id: "yandexplus", name: "Яндекс Плюс", brand: "yandexplus", brandColor: "#FC3F1D", defaultAmount: 399, defaultCurrency: "RUB", tag: "Subscription", category: "Entertainment", logoUrl: "https://thesvg.org/icons/yandexplus/default.svg" },

  // Music
  { id: "spotify", name: "Spotify", brand: "spotify", brandColor: "#1DB954", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", logoUrl: "https://thesvg.org/icons/spotify/default.svg" },
  { id: "applemusic", name: "Apple Music", brand: "applemusic", brandColor: "#FA243C", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", logoUrl: "https://thesvg.org/icons/applemusic/default.svg" },
  { id: "vkmusic", name: "VK Музыка", brand: "vkmusic", brandColor: "#0077FF", defaultAmount: 169, defaultCurrency: "RUB", tag: "Music", category: "Music", logoUrl: "https://thesvg.org/icons/vkmusic/default.svg" },

  // AI
  { id: "chatgpt", name: "ChatGPT Plus", brand: "chatgpt", brandColor: "#10A37F", defaultAmount: 20, defaultCurrency: "USD", tag: "AI", category: "AI", logoUrl: "https://thesvg.org/icons/chatgpt/default.svg" },
  { id: "midjourney", name: "Midjourney", brand: "midjourney", brandColor: "#000000", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Art", category: "AI", logoUrl: "https://thesvg.org/icons/midjourney/default.svg" },

  // Design
  { id: "figma", name: "Figma", brand: "figma", brandColor: "#F24E1E", defaultAmount: 15, defaultCurrency: "USD", tag: "Design", category: "Design", logoUrl: "https://thesvg.org/icons/figma/default.svg" },
  { id: "canva", name: "Canva Pro", brand: "canva", brandColor: "#00C4CC", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Design", category: "Design", logoUrl: "https://thesvg.org/icons/canva/default.svg" },
  { id: "adobe", name: "Adobe CC", brand: "adobe", brandColor: "#FF0000", defaultAmount: 54.99, defaultCurrency: "USD", tag: "Design", category: "Design", logoUrl: "https://thesvg.org/icons/adobe/default.svg" },

  // Productivity
  { id: "notion", name: "Notion", brand: "notion", brandColor: "#1F1F1F", defaultAmount: 8.0, defaultCurrency: "USD", tag: "Workspace", category: "Productivity", logoUrl: "https://thesvg.org/icons/notion/default.svg" },
  { id: "todoist", name: "Todoist Pro", brand: "todoist", brandColor: "#E44332", defaultAmount: 4, defaultCurrency: "USD", tag: "Tasks", category: "Productivity", logoUrl: "https://thesvg.org/icons/todoist/default.svg" },
  { id: "linear", name: "Linear", brand: "linear", brandColor: "#5E6AD2", defaultAmount: 8, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", logoUrl: "https://thesvg.org/icons/linear/default.svg" },
  { id: "slack", name: "Slack Pro", brand: "slack", brandColor: "#4A154B", defaultAmount: 7.25, defaultCurrency: "USD", tag: "Communication", category: "Productivity", logoUrl: "https://thesvg.org/icons/slack/default.svg" },
  { id: "zoom", name: "Zoom Pro", brand: "zoom", brandColor: "#2D8CFF", defaultAmount: 13.33, defaultCurrency: "USD", tag: "Communication", category: "Productivity", logoUrl: "https://thesvg.org/icons/zoom/default.svg" },
  { id: "github", name: "GitHub Pro", brand: "github", brandColor: "#181717", defaultAmount: 4, defaultCurrency: "USD", tag: "Development", category: "Productivity", logoUrl: "https://thesvg.org/icons/github/default.svg" },

  // Social
  { id: "telegram", name: "Telegram Premium", brand: "telegram", brandColor: "#229ED9", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Messaging", category: "Social", logoUrl: "https://thesvg.org/icons/telegram/default.svg" },

  // Cloud / Utilities
  { id: "icloud", name: "iCloud+", brand: "icloud", brandColor: "#3693F3", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", logoUrl: "https://thesvg.org/icons/icloud/default.svg" },
  { id: "dropbox", name: "Dropbox Plus", brand: "dropbox", brandColor: "#0061FF", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", logoUrl: "https://thesvg.org/icons/dropbox/default.svg" },
  { id: "googleone", name: "Google One", brand: "googleone", brandColor: "#4285F4", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", logoUrl: "https://thesvg.org/icons/googleone/default.svg" },
  { id: "onepassword", name: "1Password", brand: "onepassword", brandColor: "#0094F5", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Security", category: "Utilities", logoUrl: "https://thesvg.org/icons/onepassword/default.svg" },
  { id: "mts", name: "МТС Premium", brand: "mts", brandColor: "#E30611", defaultAmount: 299, defaultCurrency: "RUB", tag: "Telecom", category: "Utilities", logoUrl: "https://thesvg.org/icons/mts/default.svg" },

  // VPN
  { id: "nordvpn", name: "NordVPN", brand: "nordvpn", brandColor: "#4687FF", defaultAmount: 12.99, defaultCurrency: "USD", tag: "VPN", category: "VPN", logoUrl: "https://thesvg.org/icons/nordvpn/default.svg" },
  { id: "expressvpn", name: "ExpressVPN", brand: "expressvpn", brandColor: "#DA3940", defaultAmount: 12.95, defaultCurrency: "USD", tag: "VPN", category: "VPN", logoUrl: "https://thesvg.org/icons/expressvpn/default.svg" },

  // Games
  { id: "xbox", name: "Xbox Game Pass", brand: "xbox", brandColor: "#107C10", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", logoUrl: "https://thesvg.org/icons/xbox/default.svg" },
  { id: "playstation", name: "PS Plus", brand: "playstation", brandColor: "#003087", defaultAmount: 17.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", logoUrl: "https://thesvg.org/icons/playstation/default.svg" },

  // Health & Fitness
  { id: "duolingo", name: "Duolingo Plus", brand: "duolingo", brandColor: "#58CC02", defaultAmount: 6.99, defaultCurrency: "USD", tag: "Education", category: "Health & Fitness", logoUrl: "https://thesvg.org/icons/duolingo/default.svg" },
  { id: "strava", name: "Strava", brand: "strava", brandColor: "#FC4C02", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Fitness", category: "Health & Fitness", logoUrl: "https://thesvg.org/icons/strava/default.svg" },
  { id: "headspace", name: "Headspace", brand: "headspace", brandColor: "#F47D31", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Meditation", category: "Health & Fitness", logoUrl: "https://thesvg.org/icons/headspace/default.svg" },
];
