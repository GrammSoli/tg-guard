import type { BrandKey } from "@/types/subscription";
import { ServiceLogo } from "./ServiceLogo";

interface Props {
  brand: BrandKey;
  size?: "sm" | "md" | "lg";
  className?: string;
}

const SIZE_MAP = {
  sm: 36,
  md: 48,
  lg: 56,
} as const;

/**
 * Wrapper around ServiceLogo that provides consistent sizing
 * with sm/md/lg presets used throughout the dashboard.
 */
export function BrandIcon({ brand, size = "md", className = "" }: Props) {
  const px = SIZE_MAP[size];
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
