import type { ServiceCategory } from "./mockData";

/**
 * Maps a ServiceCategory (or the "All" tab) to its i18n key. Centralised
 * here so adding a category only touches one place. We slugify any
 * non-alphanumeric characters (e.g. "Health & Fitness" → "health_fitness")
 * to keep the JSON-file keys readable.
 */
export function categoryKey(category: "All" | ServiceCategory): string {
  if (category === "All") return "category.all";
  const slug = category
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
  return `category.${slug}`;
}
