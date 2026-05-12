import { useState } from "react";
import { brandColorFor } from "@/lib/brandLogo";
import type { BrandKey } from "@/types/subscription";

interface Props {
  brand: BrandKey;
  name: string;
  size?: number; // px
  className?: string;
}

export function MiniBrandLogo({ brand, name, size = 18, className = "" }: Props) {
  const [failed, setFailed] = useState(false);
  const color = brandColorFor(brand);
  const iconPath = `https://thesvg.org/icons/${brand}/default.svg`;
  const letter = (name || brand || "?").charAt(0).toUpperCase();
  const dim = { width: size, height: size };

  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center rounded-full ${className}`}
      style={{ ...dim, border: `2px solid ${color}`, padding: 1 }}
      aria-label={name}
    >
      {!failed ? (
        <img
          src={iconPath}
          alt={name}
          onError={() => setFailed(true)}
          className="h-full w-full rounded-full object-contain"
          loading="lazy"
        />
      ) : (
        <span
          className="flex h-full w-full items-center justify-center rounded-full font-bold text-white"
          style={{ backgroundColor: color, fontSize: size * 0.45 }}
        >
          {letter}
        </span>
      )}
    </span>
  );
}
