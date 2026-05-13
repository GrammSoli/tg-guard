import type { BrandKey } from "@/types/subscription";
import { ServiceLogo } from "./ServiceLogo";
import { COLOR_MAP, ICON_MAP } from "@/lib/customIcons";

interface Props {
  brand: BrandKey;
  size?: "sm" | "md" | "lg";
  className?: string;
  /** Custom icon overrides for "default" brand subscriptions. */
  iconName?: string;
  iconColor?: string;
}

const SIZE_MAP = {
  sm: 36,
  md: 48,
  lg: 56,
} as const;

const ICON_PX = {
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
  iconName,
  iconColor,
}: Props) {
  const px = SIZE_MAP[size];

  if (brand === "default" && iconName && iconColor) {
    const Icon = ICON_MAP[iconName];
    const colour = COLOR_MAP[iconColor];
    if (Icon && colour) {
      return (
        <div
          className={`shadow-elevated flex shrink-0 items-center justify-center rounded-2xl text-white ${colour.bg} ${className}`}
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
      rounded="2xl"
      className={`shadow-elevated ${className}`}
    />
  );
}
