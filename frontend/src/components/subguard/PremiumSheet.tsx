import { useTranslation } from "react-i18next";
import { Crown, Sparkles, Zap, X } from "lucide-react";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
  DrawerFooter,
} from "@/components/ui/drawer";
import { hapticImpact } from "@/lib/telegram";
import { usePaywallStore } from "@/stores/paywallStore";

interface PremiumSheetProps {
  open: boolean;
  onClose: () => void;
}

/**
 * Premium upgrade bottom-sheet. Shown when a free-tier user hits a paywall
 * limit (e.g. max subscriptions or max rooms). The "Upgrade" CTA currently
 * shows a coming-soon toast — swap it for real payment once Stars flow is
 * ready.
 *
 * Pricing is locale-split and admin-configurable: the prices come from
 * the paywall config (GET /api/v1/config) and the pair shown is picked
 * by the app's current i18n language, never hardcoded.
 */
export function PremiumSheet({ open, onClose }: PremiumSheetProps) {
  const { t, i18n } = useTranslation();
  const config = usePaywallStore((s) => s.config);

  // i18n.language is exactly "ru" | "en" (set in lib/i18n.ts); startsWith
  // guards against a regional tag ever slipping in.
  const isRu = i18n.language.startsWith("ru");
  const starsPrice = isRu ? config.price_stars_ru : config.price_stars_en;
  const cryptoPrice = isRu ? config.price_crypto_usd_ru : config.price_crypto_usd_en;

  const handleUpgrade = () => {
    hapticImpact("medium");
    // TODO: integrate with Telegram Stars payment
    import("sonner").then(({ toast }) => {
      toast.info(t("toast.comingSoon"));
    });
    onClose();
  };

  return (
    <Drawer open={open} onOpenChange={(v) => !v && onClose()}>
      <DrawerContent>
        <DrawerHeader className="text-center">
          <div className="mx-auto mb-3 flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-amber-400/20 to-orange-500/20">
            <Crown className="h-8 w-8 text-amber-400" />
          </div>
          <DrawerTitle className="text-xl">
            {t("premium.title")}
          </DrawerTitle>
          <DrawerDescription className="mt-1">
            {t("premium.description")}
          </DrawerDescription>
        </DrawerHeader>

        <div className="space-y-3 px-6">
          <div className="flex items-start gap-3 rounded-xl bg-surface p-3">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/15">
              <Sparkles className="h-4 w-4 text-primary" />
            </div>
            <div>
              <p className="text-sm font-semibold">{t("premium.unlimitedSubs")}</p>
              <p className="text-xs text-muted-foreground">
                {t("premium.unlimitedSubsHint")}
              </p>
            </div>
          </div>

          <div className="flex items-start gap-3 rounded-xl bg-surface p-3">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/15">
              <Zap className="h-4 w-4 text-primary" />
            </div>
            <div>
              <p className="text-sm font-semibold">{t("premium.unlimitedRooms")}</p>
              <p className="text-xs text-muted-foreground">
                {t("premium.unlimitedRoomsHint")}
              </p>
            </div>
          </div>
        </div>

        {/* Locale-split Premium price, sourced from /config — never
            hardcoded. Shows both payment options the bot supports. */}
        <div className="px-6 pt-4">
          <div className="flex items-center justify-center gap-3 rounded-xl bg-surface p-3 text-sm font-semibold">
            <span>⭐ {starsPrice} Stars</span>
            <span className="text-muted-foreground">·</span>
            <span>💵 ${cryptoPrice}</span>
          </div>
        </div>

        <DrawerFooter>
          <button
            onClick={handleUpgrade}
            className="w-full rounded-full bg-gradient-to-r from-amber-400 to-orange-500 px-6 py-3 text-sm font-bold text-black shadow-lg transition-transform active:scale-[0.97]"
          >
            {t("premium.upgrade")} · ⭐ {starsPrice}
          </button>
          <button
            onClick={onClose}
            className="w-full rounded-full px-6 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
          >
            {t("premium.later")}
          </button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
