import type { BrandKey } from "@/types/subscription";
import { ServiceLogo } from "./ServiceLogo";
import { COLOR_MAP, ICON_MAP } from "@/lib/customIcons";

interface Props {
  brand: BrandKey;
  size?: "xs" | "sm" | "md" | "lg";
  className?: string;
  /** Corner radius. Default 2xl matches AddSubscriptionSheet-style
   *  rounded-square avatars; "full" makes a circle for stacks where
   *  the surrounding ServiceLogos are circular (dashboard room card). */
  rounded?: "2xl" | "full";
  /** Custom icon overrides for "default" brand subscriptions. */
  iconName?: string;
  iconColor?: string;
  /** Explicit domain override for ServiceLogo's Brandfetch lookup. When
   *  set, wins over the catalog mapping by `brand` — used when the
   *  caller knows the user typed a hostname (e.g. "nike.com") as their
   *  custom subscription name and we want the logo right then. */
  domain?: string;
}

// xs/sm/md/lg: dashboard room-card stack uses xs (20-24px), the
// per-subscription rows use sm, AddSubscriptionSheet header uses md/lg.
const SIZE_MAP = {
  xs: 24,
  sm: 36,
  md: 48,
  lg: 56,
} as const;

const ICON_PX = {
  xs: 14,
  sm: 18,
  md: 24,
  lg: 28,
} as const;

/**
 * Avatar for a subscription. Three layers of fallback:
 *   1. brand is a known service (Netflix, Spotify, …) → ServiceLogo.
 *   2. brand === "default" AND iconName + iconColor in our allow-list →
 *      a colour-tinted circle with the lucide icon inside.
 *   3. Otherwise → ServiceLogo's letter-avatar placeholder.
 */
export function BrandIcon({
  brand,
  size = "md",
  className = "",
  rounded = "2xl",
  iconName,
  iconColor,
  domain,
}: Props) {
  const px = SIZE_MAP[size];
  const roundClass = rounded === "full" ? "rounded-full" : "rounded-2xl";

  // When the caller passes an explicit domain (typically because the
  // user typed a hostname as the custom subscription name), the
  // Brandfetch logo wins over the IconPicker selection. Lets the user
  // type "nike.com" and immediately see the Nike logo without going
  // through the catalog.
  if (domain) {
    return (
      <ServiceLogo
        brand={brand}
        name={brand}
        size={px}
        rounded={rounded}
        domain={domain}
        className={`shadow-elevated ${className}`}
      />
    );
  }

  if (brand === "default" && iconName && iconColor) {
    const Icon = ICON_MAP[iconName];
    const colour = COLOR_MAP[iconColor];
    if (Icon && colour) {
      return (
        <div
          className={`shadow-elevated flex shrink-0 items-center justify-center ${roundClass} text-white ${colour.bg} ${className}`}
          style={{ width: px, height: px }}
          aria-hidden="true"
        >
          <Icon size={ICON_PX[size]} strokeWidth={2.25} />
        </div>
      );
    }
  }

  return (
    <ServiceLogo
      brand={brand}
      name={brand}
      size={px}
      rounded={rounded}
      className={`shadow-elevated ${className}`}
    />
  );
}
