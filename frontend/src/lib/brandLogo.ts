import { POPULAR_SERVICES } from "@/lib/mockData";
import type { BrandKey } from "@/types/subscription";

const DEFAULT_COLOR = "#7C3AED";

/**
 * Map BrandKey → hex color, built once from POPULAR_SERVICES. The
 * color is used by ServiceLogo / MiniBrandLogo as the gradient
 * background of the first-letter placeholder when the Brandfetch
 * CDN fails or no clientId is configured.
 */
const COLOR_BY_BRAND: Map<string, string> = (() => {
  const m = new Map<string, string>();
  for (const svc of POPULAR_SERVICES) m.set(svc.brand, svc.brandColor);
  return m;
})();

/**
 * Returns the brand's primary hex color, or a neutral purple when the
 * brand isn't in the catalog (custom / legacy entries).
 */
export function brandColorFor(brand: BrandKey | string | undefined | null): string {
  if (!brand) return DEFAULT_COLOR;
  return COLOR_BY_BRAND.get(brand) ?? DEFAULT_COLOR;
}
