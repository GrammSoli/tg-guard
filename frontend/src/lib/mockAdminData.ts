export interface AdminStat {
  label: string;
  value: string;
  delta?: string;
}

export interface AdminCatalogService {
  id: string;
  name: string;
  category: string;
  domain: string;
  active: boolean;
}

export interface UserGrowthPoint {
  day: string;
  users: number;
}

export interface PopularServiceStat {
  name: string;
  brand: string;
  count: number;
}

export interface FunnelStep {
  label: string;
  value: number;
  color: string;
}

export interface DeepLinkStat {
  tag: string;
  clicks: number;
  botStarts: number;
  auths: number;
  cr: string;
}

// ── Live Metrics ──
export const adminKpis = {
  totalUsers: 12834,
  activeSubscriptions: 28492,
  sharedRooms: 1247,
  mrr: 18420,
};

export const liveMetrics = {
  totalUsers: 12834,
  dau: 3241,
  mau: 9872,
  donators: 487,
};

// ── User Growth ──
export const userGrowth7d: UserGrowthPoint[] = [
  { day: "Mon", users: 11820 },
  { day: "Tue", users: 11975 },
  { day: "Wed", users: 12110 },
  { day: "Thu", users: 12290 },
  { day: "Fri", users: 12455 },
  { day: "Sat", users: 12640 },
  { day: "Sun", users: 12834 },
];

// ── Popular Services (Top-10) ──
export const popularServices: PopularServiceStat[] = [
  { name: "Netflix", brand: "netflix", count: 4821 },
  { name: "Spotify", brand: "spotify", count: 3947 },
  { name: "YouTube Premium", brand: "youtube", count: 3612 },
  { name: "Telegram Premium", brand: "telegram", count: 2890 },
  { name: "iCloud+", brand: "icloud", count: 2145 },
  { name: "ChatGPT Plus", brand: "chatgpt", count: 1876 },
  { name: "Disney+", brand: "disney", count: 1543 },
  { name: "Apple Music", brand: "applemusic", count: 1320 },
  { name: "Notion", brand: "notion", count: 1105 },
  { name: "Figma", brand: "figma", count: 892 },
];

// ── Conversion Funnel ──
export const funnelSteps: FunnelStep[] = [
  { label: "Bot Starts", value: 18420, color: "hsl(var(--primary))" },
  { label: "TMA Auths", value: 12834, color: "hsl(260 60% 55%)" },
  { label: "First Subscription", value: 7291, color: "hsl(190 70% 50%)" },
  { label: "Stars Donation", value: 487, color: "hsl(45 90% 55%)" },
];

// ── Deep Links ──
export const deepLinkStats: DeepLinkStat[] = [
  { tag: "ad_telegram_channel", clicks: 4820, botStarts: 2410, auths: 1680, cr: "34.9%" },
  { tag: "ad_youtube_review", clicks: 3150, botStarts: 1260, auths: 840, cr: "26.7%" },
  { tag: "ad_twitter_promo", clicks: 1840, botStarts: 552, auths: 320, cr: "17.4%" },
  { tag: "partner_bot_swap", clicks: 960, botStarts: 384, auths: 245, cr: "25.5%" },
];

// ── Catalog ──
export const adminCatalog: AdminCatalogService[] = [
  { id: "netflix", name: "Netflix", category: "Entertainment", domain: "netflix.com", active: true },
  { id: "spotify", name: "Spotify", category: "Music", domain: "spotify.com", active: true },
  { id: "youtube", name: "YouTube Premium", category: "Entertainment", domain: "youtube.com", active: true },
  { id: "disney", name: "Disney+", category: "Entertainment", domain: "disneyplus.com", active: true },
  { id: "applemusic", name: "Apple Music", category: "Music", domain: "music.apple.com", active: true },
  { id: "notion", name: "Notion", category: "Productivity", domain: "notion.so", active: true },
  { id: "telegram", name: "Telegram Premium", category: "Social", domain: "telegram.org", active: true },
  { id: "icloud", name: "iCloud+", category: "Utilities", domain: "icloud.com", active: false },
];

export const adminGlobalSettings = {
  cpaEnabled: true,
  channelGateEnabled: false,
  targetChannel: "@subguard_official",
};
