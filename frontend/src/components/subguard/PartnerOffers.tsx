import type { PartnerOffer } from "@/types/subscription";
import { useTranslation } from "react-i18next";
import { BrandIcon } from "./BrandIcon";
import { ArrowUpRight } from "lucide-react";

interface Props {
  offers: PartnerOffer[];
  onClaim?: (offer: PartnerOffer) => void;
}

export function PartnerOffers({ offers, onClaim }: Props) {
  const { t } = useTranslation();
  if (!offers.length) return null;
  return (
    <section className="px-5">
      <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("partner.title")}
      </p>
      <div className="-mx-5 flex gap-3 overflow-x-auto px-5 pb-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        {offers.map((o) => (
          <button
            key={o.id}
            onClick={() => onClaim?.(o)}
            className="bg-surface min-w-[220px] shrink-0 rounded-2xl p-4 text-left"
          >
            <div className="flex items-start justify-between">
              <BrandIcon brand={o.brand} size="sm" />
              <span className="bg-primary/15 text-primary rounded-full px-2 py-0.5 text-[10px] font-semibold">
                {o.reward}
              </span>
            </div>
            <p className="mt-3 text-sm font-semibold">{o.name}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">{o.tagline}</p>
            <div className="text-primary mt-3 flex items-center gap-1 text-xs font-semibold">
              {t("partner.cta")} <ArrowUpRight className="h-3 w-3" />
            </div>
          </button>
        ))}
      </div>
    </section>
  );
}
