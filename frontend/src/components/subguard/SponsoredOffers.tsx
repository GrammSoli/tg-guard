import { useEffect, useRef, useState } from "react";
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
 *
 * Impressions are tracked via IntersectionObserver — fires once when ≥50% of
 * the section is visible. Clicks are tracked on CTA tap (fire-and-forget).
 */
export function SponsoredOffers() {
  const { t } = useTranslation();
  const [offers, setOffers] = useState<SponsoredOffer[]>([]);
  const sectionRef = useRef<HTMLElement>(null);
  const viewedRef = useRef(false);

  useEffect(() => {
    // `cancelled` flag plus AbortController guards against a stale
    // setState call if SponsoredOffers unmounts before the fetch
    // resolves (user switches tab, navigates away). Without it React
    // logs a "setState on unmounted component" warning in dev and
    // schedules a useless update in prod. Audit Tier-4 #2.
    let cancelled = false;
    const ac = new AbortController();
    api<SponsoredOffer[]>("/recommendations", { signal: ac.signal })
      .then((data) => {
        if (!cancelled) setOffers(data);
      })
      .catch(() => {
        if (!cancelled) setOffers([]);
      });
    return () => {
      cancelled = true;
      ac.abort();
    };
  }, []);

  // Track impressions when section scrolls into view (once per session).
  useEffect(() => {
    if (offers.length === 0 || viewedRef.current) return;

    const el = sectionRef.current;
    if (!el) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !viewedRef.current) {
          viewedRef.current = true;
          const ids = offers.map((o) => o.id);
          api("/recommendations/track/view", {
            method: "POST",
            body: { ids },
          }).catch(() => {}); // fire-and-forget
          observer.disconnect();
        }
      },
      { threshold: 0.5 },
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [offers]);

  const trackClick = (id: number) => {
    // Fire-and-forget — the click reaches the network before the
    // browser navigates (target="_blank" keeps this WebView on the
    // current URL while opening a new tab, so the in-flight fetch
    // is not aborted by navigation in practice).
    //
    // We considered navigator.sendBeacon here as a more robust
    // delivery channel for the "tab is about to navigate away" case,
    // but sendBeacon can't carry the X-Telegram-Init-Data header that
    // AuthMiddleware reads — moving auth to a query param for one
    // endpoint is a separate audit item. Audit Tier-4 #2 follow-up.
    api(`/recommendations/${id}/track/click`, { method: "POST", body: {} }).catch(
      () => {},
    );
  };

  if (offers.length === 0) return null;

  return (
    <section ref={sectionRef} className="px-5">
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
            onClick={() => trackClick(o.id)}
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
