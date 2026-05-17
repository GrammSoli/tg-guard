import { useState } from "react";
import { brandColorFor } from "@/lib/brandLogo";
import { brandfetchIcon } from "@/lib/brandfetch";
import { domainFor } from "@/lib/mockData";
import type { BrandKey } from "@/types/subscription";

interface Props {
  brand: BrandKey;
  name: string;
  size?: number;
  className?: string;
  rounded?: "full" | "xl" | "2xl";
  /** Optional domain override. When omitted, the catalog mapping in
   *  mockData.ts (domainFor) supplies the domain for the brand. */
  domain?: string;
}

/**
 * Universal service logo with guaranteed fallback.
 *
 * Source order:
 *   1. Brandfetch CDN (`cdn.brandfetch.io/{domain}?c=...`) — high
 *      quality, theme-aware, ~500 brands covered by free tier.
 *      Requires VITE_BRANDFETCH_CLIENT_ID. Domain is taken from the
 *      explicit `domain` prop or the catalog mapping for `brand`.
 *   2. Gradient first-letter tile — used when Brandfetch errors,
 *      when no clientId is configured, or when the brand has no
 *      catalog entry (legacy / custom).
 */
export function ServiceLogo({
  brand,
  name,
  size = 48,
  className = "",
  rounded = "2xl",
  domain,
}: Props) {
  const [failed, setFailed] = useState(false);
  const color = brandColorFor(brand);

  // Resolve the CDN URL: explicit domain prop wins; otherwise look up
  // the catalog. Round to the next 16-px bucket so a screen rendering
  // 18px, 20px, 22px logos all share one cache entry on the CDN.
  //
  // theme=dark looks counter-intuitive but matches Brandfetch's API
  // convention: "theme/dark" means "the variant optimised FOR a dark
  // background" — i.e. the white/light-coloured logo. Our app is
  // dark-themed end-to-end (Dashboard.tsx is bg-background which is
  // the dark surface), so we always want the light logo variant.
  // If a brand doesn't publish a dark-theme variant, Brandfetch
  // falls back to the default colour logo, which usually still reads
  // fine on dark surfaces.
  const resolvedDomain = domain ?? domainFor(brand);
  const cdnSize = Math.max(16, Math.ceil(size / 16) * 16);
  const cdnUrl = brandfetchIcon(resolvedDomain, {
    size: cdnSize,
    type: "icon",
    theme: "dark",
    fallback: "transparent",
  });

  const letter = (name || brand || "?").charAt(0).toUpperCase();
  const roundedClass =
    rounded === "full" ? "rounded-full" : rounded === "xl" ? "rounded-xl" : "rounded-2xl";

  const showImage = cdnUrl && !failed;

  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center overflow-hidden ${roundedClass} ${className}`}
      style={{ width: size, height: size }}
    >
      {showImage ? (
        <img
          src={cdnUrl}
          alt={name}
          width={size}
          height={size}
          onError={() => setFailed(true)}
          className={`h-full w-full object-contain ${roundedClass}`}
          loading="lazy"
          draggable={false}
        />
      ) : (
        <span
          className={`flex h-full w-full items-center justify-center ${roundedClass} font-bold text-white`}
          style={{
            background: `linear-gradient(135deg, ${color}, ${adjustBrightness(color, -30)})`,
            fontSize: size * 0.4,
          }}
        >
          {letter}
        </span>
      )}
    </span>
  );
}

/** Darken/lighten a hex color by `amount` (-100 to 100). */
function adjustBrightness(hex: string, amount: number): string {
  const clamp = (v: number) => Math.min(255, Math.max(0, v));
  const h = hex.replace("#", "");
  const r = clamp(parseInt(h.substring(0, 2), 16) + amount);
  const g = clamp(parseInt(h.substring(2, 4), 16) + amount);
  const b = clamp(parseInt(h.substring(4, 6), 16) + amount);
  return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
}
