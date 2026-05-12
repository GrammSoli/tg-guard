import type { BrandKey } from "@/types/subscription";

const BRAND_META: Partial<Record<BrandKey, { domain: string; color: string }>> = {
  netflix: { domain: "netflix.com", color: "#E50914" },
  spotify: { domain: "spotify.com", color: "#1DB954" },
  youtube: { domain: "youtube.com", color: "#FF0000" },
  icloud: { domain: "icloud.com", color: "#3693F3" },
  apple: { domain: "apple.com", color: "#000000" },
  applemusic: { domain: "music.apple.com", color: "#FA243C" },
  telegram: { domain: "telegram.org", color: "#229ED9" },
  disney: { domain: "disneyplus.com", color: "#0F47BA" },
  notion: { domain: "notion.so", color: "#111111" },
  chatgpt: { domain: "openai.com", color: "#10A37F" },
  midjourney: { domain: "midjourney.com", color: "#000000" },
  figma: { domain: "figma.com", color: "#F24E1E" },
  canva: { domain: "canva.com", color: "#00C4CC" },
  dropbox: { domain: "dropbox.com", color: "#0061FF" },
  googleone: { domain: "one.google.com", color: "#4285F4" },
  xbox: { domain: "xbox.com", color: "#107C10" },
  playstation: { domain: "playstation.com", color: "#003087" },
  twitch: { domain: "twitch.tv", color: "#9146FF" },
  hbomax: { domain: "max.com", color: "#002BE7" },
  crunchyroll: { domain: "crunchyroll.com", color: "#F47521" },
  nordvpn: { domain: "nordvpn.com", color: "#4687FF" },
  expressvpn: { domain: "expressvpn.com", color: "#DA3940" },
  onepassword: { domain: "1password.com", color: "#0094F5" },
  todoist: { domain: "todoist.com", color: "#E44332" },
  linear: { domain: "linear.app", color: "#5E6AD2" },
  slack: { domain: "slack.com", color: "#4A154B" },
  zoom: { domain: "zoom.us", color: "#2D8CFF" },
  duolingo: { domain: "duolingo.com", color: "#58CC02" },
  strava: { domain: "strava.com", color: "#FC4C02" },
  headspace: { domain: "headspace.com", color: "#F47D31" },
  github: { domain: "github.com", color: "#181717" },
  adobe: { domain: "adobe.com", color: "#FF0000" },
  yandexplus: { domain: "plus.yandex.ru", color: "#FC3F1D" },
  vkmusic: { domain: "music.vk.com", color: "#0077FF" },
  mts: { domain: "mts.ru", color: "#E30611" },
  megogo: { domain: "megogo.net", color: "#22B14C" },
  kinopoisk: { domain: "kinopoisk.ru", color: "#FF5500" },
};

const DEFAULT_COLOR = "#7C3AED";

export function logoUrlFor(brand: BrandKey): string {
  return `https://thesvg.org/icons/${brand}/default.svg`;
}

export function brandColorFor(brand: BrandKey): string {
  return BRAND_META[brand]?.color ?? DEFAULT_COLOR;
}
