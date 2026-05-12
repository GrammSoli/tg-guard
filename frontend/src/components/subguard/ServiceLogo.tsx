import { useState } from "react";
import { brandColorFor } from "@/lib/brandLogo";
import type { BrandKey } from "@/types/subscription";

interface Props {
  brand: BrandKey;
  name: string;
  size?: number;
  className?: string;
  rounded?: "full" | "xl" | "2xl";
}

/**
 * Universal service logo with guaranteed fallback.
 * Tries /icons/{brand}.svg first, then shows a gradient circle with the first letter.
 */
export function ServiceLogo({
  brand,
  name,
  size = 48,
  className = "",
  rounded = "2xl",
}: Props) {
  const [failed, setFailed] = useState(false);
  const color = brandColorFor(brand);
  const iconPath = `https://thesvg.org/icons/${brand}/default.svg`;
  const letter = (name || brand || "?").charAt(0).toUpperCase();
  const roundedClass =
    rounded === "full" ? "rounded-full" : rounded === "xl" ? "rounded-xl" : "rounded-2xl";

  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center overflow-hidden ${roundedClass} ${className}`}
      style={{ width: size, height: size }}
    >
      {!failed ? (
        <img
          src={iconPath}
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
