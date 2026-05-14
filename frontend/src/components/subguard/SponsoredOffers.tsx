import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ArrowUpRight } from "lucide-react";
import { api } from "@/lib/api";
import { BrandIcon } from "./BrandIcon";
import type { SponsoredOffer } from "@/types/subscription";
import type { BrandKey } from "@/types/subscription";

/**
 * SponsoredOffers fetches and displays admin-created promotional cards.
 * If the API returns an empty array (globally disabled or no matching offers),
 * the entire section is hidden.
 */
export function SponsoredOffers() {
  const { t } = useTranslation();
  const [offers, setOffers] = useState<SponsoredOffer[]>([]);

  useEffect(() => {
    api<SponsoredOffer[]>("/recommendations")
      .then(setOffers)
      .catch(() => setOffers([]));
  }, []);

  if (offers.length === 0) return null;

  return (
    <section className="px-5">
      <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("sponsored.title")}
      </p>
      <div className="-mx-5 flex gap-3 overflow-x-auto px-5 pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {offers.map((o) => (
          <a
            key={o.id}
            href={o.url}
            target="_blank"
            rel="noopener noreferrer"
            className="bg-surface min-w-[220px] shrink-0 rounded-2xl p-4 text-left no-underline"
          >
            <div className="flex items-start justify-between">
              {o.icon_name?.startsWith("http") ? (
                <img
                  src={o.icon_name}
                  alt={o.title}
                  className="h-10 w-10 shrink-0 rounded-full object-cover"
                />
              ) : (
                <BrandIcon brand={(o.icon_name || "default") as BrandKey} size="sm" />
              )}
              {o.badge_text && (
                <span className="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-semibold">
                  {o.badge_text}
                </span>
              )}
            </div>
            <p className="mt-3 text-sm font-semibold">{o.title}</p>
            {o.description && (
              <p className="mt-0.5 text-xs text-muted-foreground">{o.description}</p>
            )}
            <div className="text-primary mt-3 flex items-center gap-1 text-xs font-semibold">
              {t("sponsored.cta")} <ArrowUpRight className="h-3 w-3" />
            </div>
          </a>
        ))}
      </div>
    </section>
  );
}
