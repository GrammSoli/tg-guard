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
  services: { brand: import("@/types/subscription").BrandKey }[];
}

export const mockSharedRooms: SharedRoom[] = [
  {
    id: "r1",
    name: "Family Office",
    members: 3,
    total_per_member: 12.5,
    currency: "USD",
    services: [
      { brand: "netflix" },
      { brand: "disney" },
      { brand: "spotify" },
    ],
  },
  {
    id: "r2",
    name: "Roommates",
    members: 4,
    total_per_member: 8.75,
    currency: "USD",
    services: [
      { brand: "youtube" },
      { brand: "icloud" },
    ],
  },
  {
    id: "r3",
    name: "Study Group",
    members: 5,
    total_per_member: 4.2,
    currency: "USD",
    services: [
      { brand: "notion" },
      { brand: "applemusic" },
      { brand: "spotify" },
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
  | "Design"
  | "Education"
  | "Reading";

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
  "Education",
  "Reading",
];

export interface PopularService {
  /** Stable internal id, also the BrandKey we store in DB. */
  id: string;
  /** Display name shown to the user. */
  name: string;
  /** BrandKey alias of `id`. Kept separate so a service could later
   *  diverge (e.g. rebrand) without rewriting historic DB rows. */
  brand: import("@/types/subscription").BrandKey;
  /** Hex used by ServiceLogo as the gradient when the Brandfetch
   *  CDN fails OR is not configured. Rough approximation — the
   *  letter placeholder only shows up on misses, so exact brand
   *  match isn't critical. */
  brandColor: string;
  /** Per-period sticker price in `defaultCurrency`. Pre-filled into
   *  the Add Subscription form when the user picks this brand. */
  defaultAmount: number;
  defaultCurrency: string;
  /** Short tag shown in the picker row (e.g. "Streaming"). */
  tag: string;
  category: ServiceCategory;
  /** Canonical root domain — fed straight to cdn.brandfetch.io for
   *  the logo. Use the SHORTEST domain the brand publishes from
   *  (e.g. "spotify.com" not "accounts.spotify.com"); Brandfetch
   *  resolves subdomain → parent brand internally. */
  domain: string;
}

// Curated catalog of popular subscription services. ~200 entries
// across 14 categories, each tagged with its Brandfetch-friendly
// canonical domain. Add brands at the end of the relevant section
// — order within a category drives the picker display order.
export const POPULAR_SERVICES: PopularService[] = [
  // ── Entertainment ───────────────────────────────────────────────
  { id: "netflix", name: "Netflix", brand: "netflix", brandColor: "#E50914", defaultAmount: 15.49, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "netflix.com" },
  { id: "disney", name: "Disney+", brand: "disney", brandColor: "#0F47BA", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "disneyplus.com" },
  { id: "youtube", name: "YouTube Premium", brand: "youtube", brandColor: "#FF0000", defaultAmount: 13.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "youtube.com" },
  { id: "hbomax", name: "Max (HBO)", brand: "hbomax", brandColor: "#002BE7", defaultAmount: 15.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "max.com" },
  { id: "hulu", name: "Hulu", brand: "hulu", brandColor: "#1CE783", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "hulu.com" },
  { id: "primevideo", name: "Prime Video", brand: "primevideo", brandColor: "#00A8E1", defaultAmount: 8.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "primevideo.com" },
  { id: "appletv", name: "Apple TV+", brand: "appletv", brandColor: "#000000", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "tv.apple.com" },
  { id: "paramountplus", name: "Paramount+", brand: "paramountplus", brandColor: "#0064FF", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "paramountplus.com" },
  { id: "peacock", name: "Peacock", brand: "peacock", brandColor: "#000000", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "peacocktv.com" },
  { id: "mubi", name: "MUBI", brand: "mubi", brandColor: "#000000", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "mubi.com" },
  { id: "plex", name: "Plex Pass", brand: "plex", brandColor: "#E5A00D", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Media", category: "Entertainment", domain: "plex.tv" },
  { id: "crunchyroll", name: "Crunchyroll", brand: "crunchyroll", brandColor: "#F47521", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Anime", category: "Entertainment", domain: "crunchyroll.com" },
  { id: "twitch", name: "Twitch Turbo", brand: "twitch", brandColor: "#9146FF", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "twitch.tv" },
  { id: "discoveryplus", name: "Discovery+", brand: "discoveryplus", brandColor: "#0D9DDA", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Streaming", category: "Entertainment", domain: "discoveryplus.com" },
  { id: "espnplus", name: "ESPN+", brand: "espnplus", brandColor: "#FF0033", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Sports", category: "Entertainment", domain: "plus.espn.com" },
  { id: "fubo", name: "FuboTV", brand: "fubo", brandColor: "#FA4516", defaultAmount: 79.99, defaultCurrency: "USD", tag: "Sports", category: "Entertainment", domain: "fubo.tv" },
  { id: "vimeo", name: "Vimeo Plus", brand: "vimeo", brandColor: "#1AB7EA", defaultAmount: 7, defaultCurrency: "USD", tag: "Video", category: "Entertainment", domain: "vimeo.com" },
  { id: "kinopoisk", name: "Кинопоиск", brand: "kinopoisk", brandColor: "#FF5500", defaultAmount: 269, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "kinopoisk.ru" },
  { id: "megogo", name: "MEGOGO", brand: "megogo", brandColor: "#22B14C", defaultAmount: 199, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "megogo.net" },
  { id: "okko", name: "Okko", brand: "okko", brandColor: "#7CDB00", defaultAmount: 299, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "okko.tv" },
  { id: "ivi", name: "ivi", brand: "ivi", brandColor: "#FF6B00", defaultAmount: 399, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "ivi.ru" },
  { id: "wink", name: "Wink", brand: "wink", brandColor: "#7B61FF", defaultAmount: 299, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "wink.ru" },
  { id: "premier", name: "Premier", brand: "premier", brandColor: "#FF5722", defaultAmount: 299, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "premier.one" },
  { id: "start", name: "START", brand: "start", brandColor: "#F50059", defaultAmount: 299, defaultCurrency: "RUB", tag: "Streaming", category: "Entertainment", domain: "start.ru" },

  // ── Music ───────────────────────────────────────────────────────
  { id: "spotify", name: "Spotify", brand: "spotify", brandColor: "#1DB954", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "spotify.com" },
  { id: "applemusic", name: "Apple Music", brand: "applemusic", brandColor: "#FA243C", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "music.apple.com" },
  { id: "youtubemusic", name: "YouTube Music", brand: "youtubemusic", brandColor: "#FF0000", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "music.youtube.com" },
  { id: "tidal", name: "TIDAL", brand: "tidal", brandColor: "#000000", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "tidal.com" },
  { id: "deezer", name: "Deezer", brand: "deezer", brandColor: "#A238FF", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "deezer.com" },
  { id: "soundcloud", name: "SoundCloud Go+", brand: "soundcloud", brandColor: "#FF7700", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "soundcloud.com" },
  { id: "amazonmusic", name: "Amazon Music", brand: "amazonmusic", brandColor: "#00A8E1", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "music.amazon.com" },
  { id: "pandora", name: "Pandora Plus", brand: "pandora", brandColor: "#3668FF", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Music", category: "Music", domain: "pandora.com" },
  { id: "qobuz", name: "Qobuz Studio", brand: "qobuz", brandColor: "#0070EF", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Hi-Res", category: "Music", domain: "qobuz.com" },
  { id: "vkmusic", name: "VK Музыка", brand: "vkmusic", brandColor: "#0077FF", defaultAmount: 169, defaultCurrency: "RUB", tag: "Music", category: "Music", domain: "vk.com" },
  { id: "yandexmusic", name: "Яндекс Музыка", brand: "yandexmusic", brandColor: "#FFCC00", defaultAmount: 169, defaultCurrency: "RUB", tag: "Music", category: "Music", domain: "music.yandex.ru" },

  // ── AI ──────────────────────────────────────────────────────────
  { id: "chatgpt", name: "ChatGPT Plus", brand: "chatgpt", brandColor: "#10A37F", defaultAmount: 20, defaultCurrency: "USD", tag: "AI", category: "AI", domain: "openai.com" },
  { id: "claude", name: "Claude Pro", brand: "claude", brandColor: "#D97757", defaultAmount: 20, defaultCurrency: "USD", tag: "AI", category: "AI", domain: "claude.ai" },
  { id: "midjourney", name: "Midjourney", brand: "midjourney", brandColor: "#000000", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Art", category: "AI", domain: "midjourney.com" },
  { id: "perplexity", name: "Perplexity Pro", brand: "perplexity", brandColor: "#1B9C8F", defaultAmount: 20, defaultCurrency: "USD", tag: "AI Search", category: "AI", domain: "perplexity.ai" },
  { id: "copilot", name: "GitHub Copilot", brand: "copilot", brandColor: "#0969DA", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Code", category: "AI", domain: "github.com" },
  { id: "cursor", name: "Cursor Pro", brand: "cursor", brandColor: "#000000", defaultAmount: 20, defaultCurrency: "USD", tag: "AI Code", category: "AI", domain: "cursor.com" },
  { id: "v0", name: "v0 by Vercel", brand: "v0", brandColor: "#000000", defaultAmount: 20, defaultCurrency: "USD", tag: "AI", category: "AI", domain: "v0.dev" },
  { id: "lovable", name: "Lovable", brand: "lovable", brandColor: "#FF6E5C", defaultAmount: 20, defaultCurrency: "USD", tag: "AI", category: "AI", domain: "lovable.dev" },
  { id: "suno", name: "Suno", brand: "suno", brandColor: "#000000", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Music", category: "AI", domain: "suno.com" },
  { id: "elevenlabs", name: "ElevenLabs", brand: "elevenlabs", brandColor: "#000000", defaultAmount: 22, defaultCurrency: "USD", tag: "AI Voice", category: "AI", domain: "elevenlabs.io" },
  { id: "runway", name: "Runway", brand: "runway", brandColor: "#000000", defaultAmount: 15, defaultCurrency: "USD", tag: "AI Video", category: "AI", domain: "runwayml.com" },
  { id: "replit", name: "Replit Core", brand: "replit", brandColor: "#F26207", defaultAmount: 20, defaultCurrency: "USD", tag: "AI Code", category: "AI", domain: "replit.com" },
  { id: "characterai", name: "Character.AI+", brand: "characterai", brandColor: "#06070A", defaultAmount: 9.99, defaultCurrency: "USD", tag: "AI Chat", category: "AI", domain: "character.ai" },
  { id: "poe", name: "Poe by Quora", brand: "poe", brandColor: "#5D5CDE", defaultAmount: 19.99, defaultCurrency: "USD", tag: "AI Chat", category: "AI", domain: "poe.com" },
  { id: "you", name: "You.com Pro", brand: "you", brandColor: "#1A1A1A", defaultAmount: 20, defaultCurrency: "USD", tag: "AI Search", category: "AI", domain: "you.com" },
  { id: "gamma", name: "Gamma", brand: "gamma", brandColor: "#9333EA", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Slides", category: "AI", domain: "gamma.app" },
  { id: "notionai", name: "Notion AI", brand: "notionai", brandColor: "#1F1F1F", defaultAmount: 10, defaultCurrency: "USD", tag: "AI Writing", category: "AI", domain: "notion.so" },
  { id: "mistral", name: "Mistral Le Chat", brand: "mistral", brandColor: "#FF7000", defaultAmount: 14.99, defaultCurrency: "USD", tag: "AI Chat", category: "AI", domain: "mistral.ai" },

  // ── Cloud ───────────────────────────────────────────────────────
  { id: "icloud", name: "iCloud+", brand: "icloud", brandColor: "#3693F3", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "icloud.com" },
  { id: "googleone", name: "Google One", brand: "googleone", brandColor: "#4285F4", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "one.google.com" },
  { id: "dropbox", name: "Dropbox Plus", brand: "dropbox", brandColor: "#0061FF", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "dropbox.com" },
  { id: "onedrive", name: "OneDrive", brand: "onedrive", brandColor: "#0078D4", defaultAmount: 1.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "onedrive.live.com" },
  { id: "backblaze", name: "Backblaze", brand: "backblaze", brandColor: "#E80E0E", defaultAmount: 9, defaultCurrency: "USD", tag: "Backup", category: "Cloud", domain: "backblaze.com" },
  { id: "mega", name: "MEGA", brand: "mega", brandColor: "#D9272E", defaultAmount: 4.99, defaultCurrency: "EUR", tag: "Storage", category: "Cloud", domain: "mega.io" },
  { id: "box", name: "Box", brand: "box", brandColor: "#0061D5", defaultAmount: 10, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "box.com" },
  { id: "pcloud", name: "pCloud", brand: "pcloud", brandColor: "#17BED0", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "pcloud.com" },
  { id: "sync", name: "Sync.com", brand: "sync", brandColor: "#01568D", defaultAmount: 8, defaultCurrency: "USD", tag: "Storage", category: "Cloud", domain: "sync.com" },
  { id: "idrive", name: "IDrive", brand: "idrive", brandColor: "#0066B3", defaultAmount: 7.95, defaultCurrency: "USD", tag: "Backup", category: "Cloud", domain: "idrive.com" },
  { id: "yandexdisk", name: "Яндекс Диск", brand: "yandexdisk", brandColor: "#FFCC00", defaultAmount: 99, defaultCurrency: "RUB", tag: "Storage", category: "Cloud", domain: "disk.yandex.ru" },
  { id: "mailrucloud", name: "Mail.ru Облако", brand: "mailrucloud", brandColor: "#005FF9", defaultAmount: 149, defaultCurrency: "RUB", tag: "Storage", category: "Cloud", domain: "cloud.mail.ru" },

  // ── Productivity ────────────────────────────────────────────────
  { id: "notion", name: "Notion", brand: "notion", brandColor: "#1F1F1F", defaultAmount: 8, defaultCurrency: "USD", tag: "Workspace", category: "Productivity", domain: "notion.so" },
  { id: "linear", name: "Linear", brand: "linear", brandColor: "#5E6AD2", defaultAmount: 8, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", domain: "linear.app" },
  { id: "todoist", name: "Todoist Pro", brand: "todoist", brandColor: "#E44332", defaultAmount: 4, defaultCurrency: "USD", tag: "Tasks", category: "Productivity", domain: "todoist.com" },
  { id: "things", name: "Things", brand: "things", brandColor: "#3F92F0", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Tasks", category: "Productivity", domain: "culturedcode.com" },
  { id: "ticktick", name: "TickTick Premium", brand: "ticktick", brandColor: "#3DB46D", defaultAmount: 2.79, defaultCurrency: "USD", tag: "Tasks", category: "Productivity", domain: "ticktick.com" },
  { id: "clickup", name: "ClickUp", brand: "clickup", brandColor: "#7B68EE", defaultAmount: 7, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", domain: "clickup.com" },
  { id: "asana", name: "Asana", brand: "asana", brandColor: "#F06A6A", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", domain: "asana.com" },
  { id: "monday", name: "monday.com", brand: "monday", brandColor: "#FF3D57", defaultAmount: 9, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", domain: "monday.com" },
  { id: "trello", name: "Trello Premium", brand: "trello", brandColor: "#0079BF", defaultAmount: 10, defaultCurrency: "USD", tag: "Project Mgmt", category: "Productivity", domain: "trello.com" },
  { id: "airtable", name: "Airtable", brand: "airtable", brandColor: "#FCB400", defaultAmount: 10, defaultCurrency: "USD", tag: "Database", category: "Productivity", domain: "airtable.com" },
  { id: "evernote", name: "Evernote Personal", brand: "evernote", brandColor: "#00A82D", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "evernote.com" },
  { id: "obsidian", name: "Obsidian Sync", brand: "obsidian", brandColor: "#7C3AED", defaultAmount: 4, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "obsidian.md" },
  { id: "reflect", name: "Reflect", brand: "reflect", brandColor: "#000000", defaultAmount: 10, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "reflect.app" },
  { id: "logseq", name: "Logseq Sync", brand: "logseq", brandColor: "#85C8C8", defaultAmount: 5, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "logseq.com" },
  { id: "roam", name: "Roam Research", brand: "roam", brandColor: "#1C2A39", defaultAmount: 15, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "roamresearch.com" },
  { id: "bear", name: "Bear Pro", brand: "bear", brandColor: "#D9352B", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "bear.app" },
  { id: "craft", name: "Craft Pro", brand: "craft", brandColor: "#FF5A36", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "craft.do" },
  { id: "drafts", name: "Drafts Pro", brand: "drafts", brandColor: "#5DBAFF", defaultAmount: 1.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "getdrafts.com" },
  { id: "fantastical", name: "Fantastical", brand: "fantastical", brandColor: "#E84033", defaultAmount: 4.75, defaultCurrency: "USD", tag: "Calendar", category: "Productivity", domain: "flexibits.com" },
  { id: "microsoft365", name: "Microsoft 365", brand: "microsoft365", brandColor: "#EA3E23", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Office", category: "Productivity", domain: "microsoft.com" },
  { id: "googleworkspace", name: "Google Workspace", brand: "googleworkspace", brandColor: "#4285F4", defaultAmount: 6, defaultCurrency: "USD", tag: "Office", category: "Productivity", domain: "workspace.google.com" },
  { id: "applone", name: "Apple One", brand: "appleone", brandColor: "#000000", defaultAmount: 19.95, defaultCurrency: "USD", tag: "Bundle", category: "Productivity", domain: "apple.com" },
  { id: "zoho", name: "Zoho One", brand: "zoho", brandColor: "#E42527", defaultAmount: 37, defaultCurrency: "USD", tag: "Office", category: "Productivity", domain: "zoho.com" },
  { id: "anki", name: "AnkiWeb", brand: "anki", brandColor: "#2E7D32", defaultAmount: 24.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "ankiweb.net" },
  { id: "tana", name: "Tana", brand: "tana", brandColor: "#FF8A00", defaultAmount: 8, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "tana.inc" },
  { id: "heptabase", name: "Heptabase", brand: "heptabase", brandColor: "#000000", defaultAmount: 10.99, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "heptabase.com" },
  { id: "mem", name: "Mem", brand: "mem", brandColor: "#7B5DEC", defaultAmount: 15, defaultCurrency: "USD", tag: "Notes", category: "Productivity", domain: "mem.ai" },
  { id: "anytype", name: "Anytype", brand: "anytype", brandColor: "#000000", defaultAmount: 99, defaultCurrency: "USD", tag: "Workspace", category: "Productivity", domain: "anytype.io" },
  { id: "github", name: "GitHub Pro", brand: "github", brandColor: "#181717", defaultAmount: 4, defaultCurrency: "USD", tag: "Code", category: "Productivity", domain: "github.com" },
  { id: "gitlab", name: "GitLab Premium", brand: "gitlab", brandColor: "#FC6D26", defaultAmount: 29, defaultCurrency: "USD", tag: "Code", category: "Productivity", domain: "gitlab.com" },
  { id: "bitbucket", name: "Bitbucket", brand: "bitbucket", brandColor: "#0052CC", defaultAmount: 3, defaultCurrency: "USD", tag: "Code", category: "Productivity", domain: "bitbucket.org" },
  { id: "vercel", name: "Vercel Pro", brand: "vercel", brandColor: "#000000", defaultAmount: 20, defaultCurrency: "USD", tag: "Hosting", category: "Productivity", domain: "vercel.com" },
  { id: "netlify", name: "Netlify Pro", brand: "netlify", brandColor: "#00C7B7", defaultAmount: 19, defaultCurrency: "USD", tag: "Hosting", category: "Productivity", domain: "netlify.com" },
  { id: "cloudflare", name: "Cloudflare Pro", brand: "cloudflare", brandColor: "#F38020", defaultAmount: 25, defaultCurrency: "USD", tag: "CDN", category: "Productivity", domain: "cloudflare.com" },
  { id: "aws", name: "AWS", brand: "aws", brandColor: "#FF9900", defaultAmount: 50, defaultCurrency: "USD", tag: "Cloud", category: "Productivity", domain: "aws.amazon.com" },
  { id: "digitalocean", name: "DigitalOcean", brand: "digitalocean", brandColor: "#0080FF", defaultAmount: 12, defaultCurrency: "USD", tag: "Cloud", category: "Productivity", domain: "digitalocean.com" },
  { id: "render", name: "Render", brand: "render", brandColor: "#46E3B7", defaultAmount: 7, defaultCurrency: "USD", tag: "Cloud", category: "Productivity", domain: "render.com" },
  { id: "railway", name: "Railway", brand: "railway", brandColor: "#0B0D0E", defaultAmount: 5, defaultCurrency: "USD", tag: "Cloud", category: "Productivity", domain: "railway.app" },
  { id: "fly", name: "Fly.io", brand: "fly", brandColor: "#7B3FE4", defaultAmount: 5, defaultCurrency: "USD", tag: "Cloud", category: "Productivity", domain: "fly.io" },
  { id: "supabase", name: "Supabase Pro", brand: "supabase", brandColor: "#3ECF8E", defaultAmount: 25, defaultCurrency: "USD", tag: "Database", category: "Productivity", domain: "supabase.com" },
  { id: "firebase", name: "Firebase", brand: "firebase", brandColor: "#FFCA28", defaultAmount: 25, defaultCurrency: "USD", tag: "BaaS", category: "Productivity", domain: "firebase.google.com" },
  { id: "planetscale", name: "PlanetScale", brand: "planetscale", brandColor: "#000000", defaultAmount: 39, defaultCurrency: "USD", tag: "Database", category: "Productivity", domain: "planetscale.com" },
  { id: "sentry", name: "Sentry", brand: "sentry", brandColor: "#362D59", defaultAmount: 26, defaultCurrency: "USD", tag: "Monitoring", category: "Productivity", domain: "sentry.io" },
  { id: "datadog", name: "Datadog", brand: "datadog", brandColor: "#632CA6", defaultAmount: 15, defaultCurrency: "USD", tag: "Monitoring", category: "Productivity", domain: "datadoghq.com" },
  { id: "jetbrains", name: "JetBrains All Products", brand: "jetbrains", brandColor: "#000000", defaultAmount: 24.90, defaultCurrency: "USD", tag: "IDE", category: "Productivity", domain: "jetbrains.com" },
  { id: "postman", name: "Postman", brand: "postman", brandColor: "#FF6C37", defaultAmount: 14, defaultCurrency: "USD", tag: "API", category: "Productivity", domain: "postman.com" },
  { id: "grammarly", name: "Grammarly Premium", brand: "grammarly", brandColor: "#15C39A", defaultAmount: 12, defaultCurrency: "USD", tag: "Writing", category: "Productivity", domain: "grammarly.com" },
  { id: "deepl", name: "DeepL Pro", brand: "deepl", brandColor: "#0F2B46", defaultAmount: 7.49, defaultCurrency: "EUR", tag: "Translate", category: "Productivity", domain: "deepl.com" },
  { id: "loom", name: "Loom", brand: "loom", brandColor: "#625DF5", defaultAmount: 15, defaultCurrency: "USD", tag: "Video", category: "Productivity", domain: "loom.com" },
  { id: "krisp", name: "Krisp", brand: "krisp", brandColor: "#5750FF", defaultAmount: 8, defaultCurrency: "USD", tag: "Audio", category: "Productivity", domain: "krisp.ai" },
  { id: "zapier", name: "Zapier", brand: "zapier", brandColor: "#FF4F00", defaultAmount: 19.99, defaultCurrency: "USD", tag: "Automation", category: "Productivity", domain: "zapier.com" },
  { id: "make", name: "Make", brand: "make", brandColor: "#6D00CC", defaultAmount: 9, defaultCurrency: "USD", tag: "Automation", category: "Productivity", domain: "make.com" },
  { id: "ifttt", name: "IFTTT Pro", brand: "ifttt", brandColor: "#000000", defaultAmount: 5, defaultCurrency: "USD", tag: "Automation", category: "Productivity", domain: "ifttt.com" },

  // ── Design ──────────────────────────────────────────────────────
  { id: "figma", name: "Figma", brand: "figma", brandColor: "#F24E1E", defaultAmount: 15, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "figma.com" },
  { id: "adobe", name: "Adobe Creative Cloud", brand: "adobe", brandColor: "#FF0000", defaultAmount: 54.99, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "adobe.com" },
  { id: "canva", name: "Canva Pro", brand: "canva", brandColor: "#00C4CC", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "canva.com" },
  { id: "framer", name: "Framer", brand: "framer", brandColor: "#0055FF", defaultAmount: 20, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "framer.com" },
  { id: "webflow", name: "Webflow", brand: "webflow", brandColor: "#4353FF", defaultAmount: 14, defaultCurrency: "USD", tag: "Web", category: "Design", domain: "webflow.com" },
  { id: "sketch", name: "Sketch", brand: "sketch", brandColor: "#FDB300", defaultAmount: 10, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "sketch.com" },
  { id: "spline", name: "Spline", brand: "spline", brandColor: "#E6E6FE", defaultAmount: 9, defaultCurrency: "USD", tag: "3D", category: "Design", domain: "spline.design" },
  { id: "procreate", name: "Procreate Dreams", brand: "procreate", brandColor: "#000000", defaultAmount: 19.99, defaultCurrency: "USD", tag: "Art", category: "Design", domain: "procreate.com" },
  { id: "affinity", name: "Affinity V2", brand: "affinity", brandColor: "#134881", defaultAmount: 169.99, defaultCurrency: "USD", tag: "Design", category: "Design", domain: "affinity.serif.com" },
  { id: "linearicons", name: "Linearicons", brand: "linearicons", brandColor: "#5E6AD2", defaultAmount: 19, defaultCurrency: "USD", tag: "Icons", category: "Design", domain: "linearicons.com" },

  // ── Social / Communication ──────────────────────────────────────
  { id: "telegram", name: "Telegram Premium", brand: "telegram", brandColor: "#229ED9", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Messaging", category: "Social", domain: "telegram.org" },
  { id: "discord", name: "Discord Nitro", brand: "discord", brandColor: "#5865F2", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Voice", category: "Social", domain: "discord.com" },
  { id: "slack", name: "Slack Pro", brand: "slack", brandColor: "#4A154B", defaultAmount: 7.25, defaultCurrency: "USD", tag: "Team", category: "Social", domain: "slack.com" },
  { id: "zoom", name: "Zoom Pro", brand: "zoom", brandColor: "#2D8CFF", defaultAmount: 13.33, defaultCurrency: "USD", tag: "Meetings", category: "Social", domain: "zoom.us" },
  { id: "msteams", name: "Microsoft Teams", brand: "msteams", brandColor: "#5059C9", defaultAmount: 4, defaultCurrency: "USD", tag: "Team", category: "Social", domain: "teams.microsoft.com" },
  { id: "x", name: "X Premium", brand: "x", brandColor: "#000000", defaultAmount: 8, defaultCurrency: "USD", tag: "Social", category: "Social", domain: "x.com" },
  { id: "linkedin", name: "LinkedIn Premium", brand: "linkedin", brandColor: "#0A66C2", defaultAmount: 29.99, defaultCurrency: "USD", tag: "Social", category: "Social", domain: "linkedin.com" },
  { id: "around", name: "Around", brand: "around", brandColor: "#FF6633", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Meetings", category: "Social", domain: "around.co" },

  // ── Utilities / Password / Security ─────────────────────────────
  { id: "onepassword", name: "1Password", brand: "onepassword", brandColor: "#0094F5", defaultAmount: 2.99, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "1password.com" },
  { id: "lastpass", name: "LastPass Premium", brand: "lastpass", brandColor: "#D32D27", defaultAmount: 3, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "lastpass.com" },
  { id: "bitwarden", name: "Bitwarden Premium", brand: "bitwarden", brandColor: "#175DDC", defaultAmount: 0.83, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "bitwarden.com" },
  { id: "dashlane", name: "Dashlane", brand: "dashlane", brandColor: "#0E2DAF", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "dashlane.com" },
  { id: "nordpass", name: "NordPass", brand: "nordpass", brandColor: "#4687FF", defaultAmount: 2.49, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "nordpass.com" },
  { id: "protonpass", name: "Proton Pass", brand: "protonpass", brandColor: "#6D4AFF", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Password", category: "Utilities", domain: "proton.me" },
  { id: "protonmail", name: "Proton Mail", brand: "protonmail", brandColor: "#6D4AFF", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Email", category: "Utilities", domain: "proton.me" },
  { id: "fastmail", name: "Fastmail", brand: "fastmail", brandColor: "#296FAA", defaultAmount: 5, defaultCurrency: "USD", tag: "Email", category: "Utilities", domain: "fastmail.com" },
  { id: "hey", name: "HEY by 37signals", brand: "hey", brandColor: "#5522FA", defaultAmount: 99, defaultCurrency: "USD", tag: "Email", category: "Utilities", domain: "hey.com" },
  { id: "adguard", name: "AdGuard", brand: "adguard", brandColor: "#68BC71", defaultAmount: 2.49, defaultCurrency: "USD", tag: "Privacy", category: "Utilities", domain: "adguard.com" },
  { id: "mts", name: "МТС Premium", brand: "mts", brandColor: "#E30611", defaultAmount: 299, defaultCurrency: "RUB", tag: "Telecom", category: "Utilities", domain: "mts.ru" },
  { id: "yandexplus", name: "Яндекс Плюс", brand: "yandexplus", brandColor: "#FC3F1D", defaultAmount: 399, defaultCurrency: "RUB", tag: "Bundle", category: "Utilities", domain: "plus.yandex.ru" },
  { id: "sberprime", name: "СберПрайм", brand: "sberprime", brandColor: "#21A038", defaultAmount: 199, defaultCurrency: "RUB", tag: "Bundle", category: "Utilities", domain: "sberbank.ru" },
  { id: "ozonpremium", name: "OZON Premium", brand: "ozonpremium", brandColor: "#005BFF", defaultAmount: 299, defaultCurrency: "RUB", tag: "Bundle", category: "Utilities", domain: "ozon.ru" },
  { id: "vkcombo", name: "VK Combo", brand: "vkcombo", brandColor: "#0077FF", defaultAmount: 199, defaultCurrency: "RUB", tag: "Bundle", category: "Utilities", domain: "vk.com" },
  { id: "tinkoffpro", name: "Tinkoff Pro", brand: "tinkoffpro", brandColor: "#FFDD2D", defaultAmount: 299, defaultCurrency: "RUB", tag: "Bundle", category: "Utilities", domain: "tinkoff.ru" },
  { id: "tele2premium", name: "Tele2 Premium", brand: "tele2premium", brandColor: "#1F1F1F", defaultAmount: 249, defaultCurrency: "RUB", tag: "Telecom", category: "Utilities", domain: "tele2.ru" },
  { id: "beeline", name: "Beeline Up", brand: "beeline", brandColor: "#FFCC00", defaultAmount: 199, defaultCurrency: "RUB", tag: "Telecom", category: "Utilities", domain: "beeline.ru" },

  // ── VPN ─────────────────────────────────────────────────────────
  { id: "nordvpn", name: "NordVPN", brand: "nordvpn", brandColor: "#4687FF", defaultAmount: 12.99, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "nordvpn.com" },
  { id: "expressvpn", name: "ExpressVPN", brand: "expressvpn", brandColor: "#DA3940", defaultAmount: 12.95, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "expressvpn.com" },
  { id: "surfshark", name: "Surfshark", brand: "surfshark", brandColor: "#1EBFBF", defaultAmount: 12.95, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "surfshark.com" },
  { id: "protonvpn", name: "Proton VPN", brand: "protonvpn", brandColor: "#6D4AFF", defaultAmount: 9.99, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "protonvpn.com" },
  { id: "mullvad", name: "Mullvad", brand: "mullvad", brandColor: "#FFCD00", defaultAmount: 5, defaultCurrency: "EUR", tag: "VPN", category: "VPN", domain: "mullvad.net" },
  { id: "windscribe", name: "Windscribe", brand: "windscribe", brandColor: "#41C9F2", defaultAmount: 9, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "windscribe.com" },
  { id: "ivpn", name: "IVPN", brand: "ivpn", brandColor: "#3B3D70", defaultAmount: 6, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "ivpn.net" },
  { id: "pia", name: "Private Internet Access", brand: "pia", brandColor: "#6BCD45", defaultAmount: 11.95, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "privateinternetaccess.com" },
  { id: "cyberghost", name: "CyberGhost", brand: "cyberghost", brandColor: "#FFB000", defaultAmount: 12.99, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "cyberghostvpn.com" },
  { id: "adguardvpn", name: "AdGuard VPN", brand: "adguardvpn", brandColor: "#68BC71", defaultAmount: 11.99, defaultCurrency: "USD", tag: "VPN", category: "VPN", domain: "adguard-vpn.com" },

  // ── Games ───────────────────────────────────────────────────────
  { id: "xbox", name: "Xbox Game Pass Ultimate", brand: "xbox", brandColor: "#107C10", defaultAmount: 16.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "xbox.com" },
  { id: "playstation", name: "PS Plus Premium", brand: "playstation", brandColor: "#003087", defaultAmount: 17.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "playstation.com" },
  { id: "nintendoswitch", name: "Nintendo Switch Online", brand: "nintendoswitch", brandColor: "#E60012", defaultAmount: 3.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "nintendo.com" },
  { id: "applearcade", name: "Apple Arcade", brand: "applearcade", brandColor: "#000000", defaultAmount: 6.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "apple.com" },
  { id: "eaplay", name: "EA Play", brand: "eaplay", brandColor: "#FF4747", defaultAmount: 4.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "ea.com" },
  { id: "ubisoftplus", name: "Ubisoft+", brand: "ubisoftplus", brandColor: "#0070FF", defaultAmount: 17.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "ubisoft.com" },
  { id: "geforcenow", name: "GeForce NOW", brand: "geforcenow", brandColor: "#76B900", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Cloud Gaming", category: "Games", domain: "geforcenow.com" },
  { id: "humblechoice", name: "Humble Choice", brand: "humblechoice", brandColor: "#CC2929", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Gaming", category: "Games", domain: "humblebundle.com" },
  { id: "luna", name: "Amazon Luna", brand: "luna", brandColor: "#9333EA", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Cloud Gaming", category: "Games", domain: "luna.amazon.com" },

  // ── Education ───────────────────────────────────────────────────
  { id: "duolingo", name: "Duolingo Super", brand: "duolingo", brandColor: "#58CC02", defaultAmount: 6.99, defaultCurrency: "USD", tag: "Languages", category: "Education", domain: "duolingo.com" },
  { id: "babbel", name: "Babbel", brand: "babbel", brandColor: "#FF6600", defaultAmount: 13.95, defaultCurrency: "USD", tag: "Languages", category: "Education", domain: "babbel.com" },
  { id: "coursera", name: "Coursera Plus", brand: "coursera", brandColor: "#0056D2", defaultAmount: 59, defaultCurrency: "USD", tag: "Courses", category: "Education", domain: "coursera.org" },
  { id: "udemy", name: "Udemy Personal Plan", brand: "udemy", brandColor: "#A435F0", defaultAmount: 16.58, defaultCurrency: "USD", tag: "Courses", category: "Education", domain: "udemy.com" },
  { id: "linkedinlearning", name: "LinkedIn Learning", brand: "linkedinlearning", brandColor: "#0A66C2", defaultAmount: 39.99, defaultCurrency: "USD", tag: "Courses", category: "Education", domain: "learning.linkedin.com" },
  { id: "masterclass", name: "MasterClass", brand: "masterclass", brandColor: "#000000", defaultAmount: 15, defaultCurrency: "USD", tag: "Courses", category: "Education", domain: "masterclass.com" },
  { id: "skillshare", name: "Skillshare", brand: "skillshare", brandColor: "#002333", defaultAmount: 13.99, defaultCurrency: "USD", tag: "Courses", category: "Education", domain: "skillshare.com" },
  { id: "brilliant", name: "Brilliant", brand: "brilliant", brandColor: "#FFB400", defaultAmount: 24.99, defaultCurrency: "USD", tag: "Stem", category: "Education", domain: "brilliant.org" },
  { id: "codecademy", name: "Codecademy Pro", brand: "codecademy", brandColor: "#1F4056", defaultAmount: 19.99, defaultCurrency: "USD", tag: "Code", category: "Education", domain: "codecademy.com" },
  { id: "pluralsight", name: "Pluralsight", brand: "pluralsight", brandColor: "#F15B2A", defaultAmount: 29, defaultCurrency: "USD", tag: "Code", category: "Education", domain: "pluralsight.com" },
  { id: "frontendmasters", name: "Frontend Masters", brand: "frontendmasters", brandColor: "#C30C16", defaultAmount: 39, defaultCurrency: "USD", tag: "Code", category: "Education", domain: "frontendmasters.com" },
  { id: "memrise", name: "Memrise", brand: "memrise", brandColor: "#FF8800", defaultAmount: 8.49, defaultCurrency: "USD", tag: "Languages", category: "Education", domain: "memrise.com" },
  { id: "datacamp", name: "DataCamp", brand: "datacamp", brandColor: "#03EF62", defaultAmount: 13.50, defaultCurrency: "USD", tag: "Data", category: "Education", domain: "datacamp.com" },

  // ── Health & Fitness ────────────────────────────────────────────
  { id: "calm", name: "Calm", brand: "calm", brandColor: "#2C73CC", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Meditation", category: "Health & Fitness", domain: "calm.com" },
  { id: "headspace", name: "Headspace", brand: "headspace", brandColor: "#F47D31", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Meditation", category: "Health & Fitness", domain: "headspace.com" },
  { id: "strava", name: "Strava Premium", brand: "strava", brandColor: "#FC4C02", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Fitness", category: "Health & Fitness", domain: "strava.com" },
  { id: "whoop", name: "WHOOP", brand: "whoop", brandColor: "#FFFFFF", defaultAmount: 30, defaultCurrency: "USD", tag: "Wearable", category: "Health & Fitness", domain: "whoop.com" },
  { id: "applefitness", name: "Apple Fitness+", brand: "applefitness", brandColor: "#34C759", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Workout", category: "Health & Fitness", domain: "apple.com" },
  { id: "peloton", name: "Peloton App", brand: "peloton", brandColor: "#000000", defaultAmount: 12.99, defaultCurrency: "USD", tag: "Workout", category: "Health & Fitness", domain: "onepeloton.com" },
  { id: "myfitnesspal", name: "MyFitnessPal Premium", brand: "myfitnesspal", brandColor: "#0072CE", defaultAmount: 19.99, defaultCurrency: "USD", tag: "Nutrition", category: "Health & Fitness", domain: "myfitnesspal.com" },
  { id: "fitbit", name: "Fitbit Premium", brand: "fitbit", brandColor: "#00B0B9", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Wearable", category: "Health & Fitness", domain: "fitbit.com" },
  { id: "nike", name: "Nike Training Club", brand: "nike", brandColor: "#000000", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Workout", category: "Health & Fitness", domain: "nike.com" },
  { id: "centr", name: "Centr", brand: "centr", brandColor: "#000000", defaultAmount: 19.99, defaultCurrency: "USD", tag: "Workout", category: "Health & Fitness", domain: "centr.com" },

  // ── Reading ─────────────────────────────────────────────────────
  { id: "audible", name: "Audible", brand: "audible", brandColor: "#F8991C", defaultAmount: 14.95, defaultCurrency: "USD", tag: "Audiobooks", category: "Reading", domain: "audible.com" },
  { id: "kindleunlimited", name: "Kindle Unlimited", brand: "kindleunlimited", brandColor: "#FF9900", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Books", category: "Reading", domain: "amazon.com" },
  { id: "storytel", name: "Storytel", brand: "storytel", brandColor: "#FF5C28", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Audiobooks", category: "Reading", domain: "storytel.com" },
  { id: "scribd", name: "Scribd", brand: "scribd", brandColor: "#1A263A", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Books", category: "Reading", domain: "scribd.com" },
  { id: "blinkist", name: "Blinkist", brand: "blinkist", brandColor: "#0CCFB5", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Summaries", category: "Reading", domain: "blinkist.com" },
  { id: "readwise", name: "Readwise", brand: "readwise", brandColor: "#000000", defaultAmount: 7.99, defaultCurrency: "USD", tag: "Highlights", category: "Reading", domain: "readwise.io" },
  { id: "substack", name: "Substack (avg)", brand: "substack", brandColor: "#FF6719", defaultAmount: 5, defaultCurrency: "USD", tag: "Newsletter", category: "Reading", domain: "substack.com" },
  { id: "litres", name: "ЛитРес", brand: "litres", brandColor: "#E30613", defaultAmount: 299, defaultCurrency: "RUB", tag: "Books", category: "Reading", domain: "litres.ru" },
  { id: "nyt", name: "New York Times", brand: "nyt", brandColor: "#000000", defaultAmount: 17, defaultCurrency: "USD", tag: "News", category: "Reading", domain: "nytimes.com" },
  { id: "wsj", name: "Wall Street Journal", brand: "wsj", brandColor: "#0080C3", defaultAmount: 38.99, defaultCurrency: "USD", tag: "News", category: "Reading", domain: "wsj.com" },
  { id: "bloomberg", name: "Bloomberg", brand: "bloomberg", brandColor: "#000000", defaultAmount: 34.99, defaultCurrency: "USD", tag: "News", category: "Reading", domain: "bloomberg.com" },
  { id: "ft", name: "Financial Times", brand: "ft", brandColor: "#FFF1E5", defaultAmount: 75, defaultCurrency: "USD", tag: "News", category: "Reading", domain: "ft.com" },
  { id: "wired", name: "Wired", brand: "wired", brandColor: "#000000", defaultAmount: 5, defaultCurrency: "USD", tag: "Magazine", category: "Reading", domain: "wired.com" },
  { id: "theatlantic", name: "The Atlantic", brand: "theatlantic", brandColor: "#000000", defaultAmount: 11.99, defaultCurrency: "USD", tag: "Magazine", category: "Reading", domain: "theatlantic.com" },
  { id: "economist", name: "The Economist", brand: "economist", brandColor: "#E3120B", defaultAmount: 24.50, defaultCurrency: "USD", tag: "News", category: "Reading", domain: "economist.com" },
  { id: "medium", name: "Medium", brand: "medium", brandColor: "#000000", defaultAmount: 5, defaultCurrency: "USD", tag: "Blog", category: "Reading", domain: "medium.com" },

  // ── Finance ─────────────────────────────────────────────────────
  { id: "robinhood", name: "Robinhood Gold", brand: "robinhood", brandColor: "#00C805", defaultAmount: 5, defaultCurrency: "USD", tag: "Investing", category: "Finance", domain: "robinhood.com" },
  { id: "ynab", name: "YNAB", brand: "ynab", brandColor: "#3B5DCC", defaultAmount: 14.99, defaultCurrency: "USD", tag: "Budget", category: "Finance", domain: "ynab.com" },
  { id: "splitwise", name: "Splitwise Pro", brand: "splitwise", brandColor: "#1ABC9C", defaultAmount: 3, defaultCurrency: "USD", tag: "Split", category: "Finance", domain: "splitwise.com" },
  { id: "wise", name: "Wise Account", brand: "wise", brandColor: "#9FE870", defaultAmount: 0, defaultCurrency: "USD", tag: "Banking", category: "Finance", domain: "wise.com" },
  { id: "revolut", name: "Revolut Premium", brand: "revolut", brandColor: "#0666EB", defaultAmount: 9.99, defaultCurrency: "USD", tag: "Banking", category: "Finance", domain: "revolut.com" },
  { id: "monzo", name: "Monzo Plus", brand: "monzo", brandColor: "#FF4F40", defaultAmount: 5, defaultCurrency: "GBP", tag: "Banking", category: "Finance", domain: "monzo.com" },
];

/**
 * Map BrandKey → canonical domain, built once from POPULAR_SERVICES.
 * Used by ServiceLogo (and any other caller that needs to construct a
 * Brandfetch URL from a stored brand string).
 *
 * Returns undefined for brands not in the catalog (legacy entries,
 * custom services with brand="default"). The caller should
 * letter-fallback in that case.
 */
const DOMAIN_BY_BRAND: Map<string, string> = (() => {
  const m = new Map<string, string>();
  for (const svc of POPULAR_SERVICES) m.set(svc.brand, svc.domain);
  return m;
})();

export function domainFor(brand: string | undefined | null): string | undefined {
  if (!brand) return undefined;
  return DOMAIN_BY_BRAND.get(brand);
}
