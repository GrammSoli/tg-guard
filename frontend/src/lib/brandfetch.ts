/**
 * Brandfetch Logo Link helper.
 *
 * Docs: https://docs.brandfetch.com/logo-api/parameters
 *
 * URL shape:
 *   https://cdn.brandfetch.io/{domain}/w/{w}/h/{h}/theme/{t}/type/{T}/fallback/{f}?c={clientId}
 *
 * Free tier: 500k requests/month, no attribution required. The clientId
 * is read from VITE_BRANDFETCH_CLIENT_ID; without it the helper returns
 * null so ServiceLogo can fall back to its letter-placeholder.
 */

const CDN = "https://cdn.brandfetch.io";

const CLIENT_ID = (import.meta.env.VITE_BRANDFETCH_CLIENT_ID as string | undefined) ?? "";

export interface BrandfetchIconOpts {
  /** Pixel size — usually we render a square, so width = height. */
  size?: number;
  /** Logo variant optimised for the target background.
   *  - "dark"  → light/white logo, intended for use ON a dark
   *    background. Counter-intuitive naming but it's Brandfetch's
   *    convention (matches CSS color-scheme).
   *  - "light" → dark logo, intended for use ON a light background.
   *  When omitted, Brandfetch serves the default colour variant. */
  theme?: "light" | "dark";
  /** logo (full wordmark) | icon (compact square) | symbol (icon only).
   *  ServiceLogo uses "icon" — it's the square avatar style we want
   *  next to subscription names. */
  type?: "logo" | "icon" | "symbol";
  /** What to serve when Brandfetch has no asset for the domain.
   *  "lettermark" is the most useful at scale — fills the missing slot
   *  with a colored first-letter tile instead of a broken image. */
  fallback?: "lettermark" | "transparent" | "brandfetch" | "404";
}

/**
 * Build a Brandfetch CDN URL for `domain`. Returns null when:
 *  - VITE_BRANDFETCH_CLIENT_ID is unset (consumer should letter-fallback)
 *  - `domain` is empty
 *
 * The path segments are emitted only when their option diverges from
 * Brandfetch's default — keeps URLs short and CDN-cacheable.
 */
export function brandfetchIcon(
  domain: string | null | undefined,
  opts: BrandfetchIconOpts = {},
): string | null {
  if (!CLIENT_ID || !domain) return null;

  const parts: string[] = [domain];
  if (opts.size && opts.size > 0) {
    parts.push("w", String(opts.size));
    parts.push("h", String(opts.size));
  }
  if (opts.theme) parts.push("theme", opts.theme);
  if (opts.type) parts.push("type", opts.type);
  if (opts.fallback) parts.push("fallback", opts.fallback);

  return `${CDN}/${parts.join("/")}?c=${encodeURIComponent(CLIENT_ID)}`;
}

/** True iff a Brandfetch clientId is configured. Components can branch
 *  on this to skip allocating <img>s that they know will fail. */
export function brandfetchEnabled(): boolean {
  return CLIENT_ID.length > 0;
}

// Hostname-shaped: `label`, optional `.label`s, dot required, no scheme,
// no path. Matches "nike.com", "music.apple.com", "vk.ru"; rejects bare
// names ("netflix"), URLs ("https://..."), and random text.
const DOMAIN_RE = /^[a-z0-9-]+(\.[a-z0-9-]+)+$/i;

/**
 * Is this string shaped like a hostname operators / users would type
 * into an "icon" or "brand" field, where pulling it through Brandfetch
 * would Just Work? Used by the Add Subscription / Sponsored Offer
 * forms to auto-resolve a logo when the user types something like
 * "nike.com" instead of picking from the catalog.
 */
export function looksLikeDomain(s: string | null | undefined): boolean {
  if (!s) return false;
  // Trim, lowercase, strip trailing slash so the regex stays simple.
  const v = s.trim().toLowerCase().replace(/\/+$/, "");
  return DOMAIN_RE.test(v);
}

/**
 * For custom-brand subscriptions / services (brand="default"), if the
 * user-typed `name` is a hostname, return it normalised — to be passed
 * as the `domain` prop on BrandIcon / ServiceLogo so the avatar
 * renders the real Brandfetch logo instead of the IconPicker
 * placeholder. Returns undefined when the name isn't a domain OR the
 * brand is anything other than "default" (those go through the
 * catalog's brand-domain map already).
 */
export function domainHintFromName(
  brand: string | undefined | null,
  name: string | undefined | null,
): string | undefined {
  if (brand !== "default") return undefined;
  if (!looksLikeDomain(name)) return undefined;
  return (name as string).trim().toLowerCase().replace(/\/+$/, "");
}
