import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Crown, Sparkles, Zap } from "lucide-react";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
  DrawerFooter,
} from "@/components/ui/drawer";
import { hapticImpact, hapticNotification, openInvoice, openTelegramLink } from "@/lib/telegram";
import { usePaywallStore } from "@/stores/paywallStore";
import { useSettingsStore } from "@/stores/settingsStore";
import { api } from "@/lib/api";

interface PremiumSheetProps {
  open: boolean;
  onClose: () => void;
}

/**
 * Premium upgrade bottom-sheet. Two states:
 *
 *  - Already a donator → a confirmation screen (crown, thank-you copy,
 *    a single Close button). No feature cards, no payment buttons.
 *  - Free user → the upgrade screen: feature cards + two payment
 *    buttons. Telegram Stars is the primary CTA (amber gradient);
 *    Crypto Pay is the secondary, quieter outline button so the two
 *    don't fight for attention.
 *
 * Pricing is locale-split and admin-configurable: the prices come from
 * the paywall config (GET /api/v1/config) and the pair shown is picked
 * by the app's current i18n language, never hardcoded.
 */
export function PremiumSheet({ open, onClose }: PremiumSheetProps) {
  const { t, i18n } = useTranslation();
  const config = usePaywallStore((s) => s.config);
  const fetchProfile = useSettingsStore((s) => s.fetchProfile);
  // is_donator from /me — settingsStore maps it onto settings.isSubscribed.
  const isDonator = useSettingsStore((s) => s.settings.isSubscribed);
  const [loadingStars, setLoadingStars] = useState(false);
  const [loadingCrypto, setLoadingCrypto] = useState(false);

  const isRu = i18n.language.startsWith("ru");
  const starsPrice = isRu ? config.price_stars_ru : config.price_stars_en;
  const cryptoPrice = isRu ? config.price_crypto_usd_ru : config.price_crypto_usd_en;

  // ── Stars payment flow ────────────────────────────────────
  const handleStars = async () => {
    hapticImpact("medium");
    setLoadingStars(true);

    try {
      const { invoice_url } = await api<{ invoice_url: string }>(
        "/payments/stars",
        { method: "POST" },
      );

      const status = await openInvoice(invoice_url);

      if (status === "paid") {
        hapticNotification("success");
        await fetchProfile();
        const { toast } = await import("sonner");
        toast.success(t("premium.success_toast"));
        onClose();
      }
    } catch (err) {
      console.error("[PremiumSheet] stars error:", err);
      const { toast } = await import("sonner");
      toast.error(t("premium.payment_failed"));
    } finally {
      setLoadingStars(false);
    }
  };

  // ── Crypto Pay flow ───────────────────────────────────────
  const handleCrypto = async () => {
    hapticImpact("medium");
    setLoadingCrypto(true);

    try {
      const { invoice_url } = await api<{ invoice_url: string }>(
        "/payments/crypto",
        { method: "POST" },
      );

      // Open the CryptoBot link inside Telegram. The webhook will
      // activate premium on the backend; user will get a TG message.
      openTelegramLink(invoice_url);

      const { toast } = await import("sonner");
      toast.info(t("premium.crypto_processing"));
      onClose();
    } catch (err) {
      console.error("[PremiumSheet] crypto error:", err);
      const { toast } = await import("sonner");
      toast.error(t("premium.payment_failed"));
    } finally {
      setLoadingCrypto(false);
    }
  };

  const loading = loadingStars || loadingCrypto;

  return (
    <Drawer open={open} onOpenChange={(v) => !v && onClose()}>
      <DrawerContent>
        {isDonator ? (
          // ── Already-active state ────────────────────────────
          <>
            <DrawerHeader className="text-center">
              <div className="mx-auto mb-3 flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-amber-400/20 to-orange-500/20">
                <Crown className="h-8 w-8 text-amber-400" />
              </div>
              <DrawerTitle className="text-xl">
                {t("premium.already_active_title")}
              </DrawerTitle>
              <DrawerDescription className="mt-1">
                {t("premium.already_active_desc")}
              </DrawerDescription>
            </DrawerHeader>

            <DrawerFooter>
              <button
                onClick={onClose}
                className="w-full rounded-full bg-primary px-6 py-3 text-sm font-bold text-primary-foreground shadow-lg transition-transform active:scale-[0.97]"
              >
                {t("premium.close")}
              </button>
            </DrawerFooter>
          </>
        ) : (
          // ── Upgrade state ───────────────────────────────────
          <>
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

            <DrawerFooter className="gap-3">
              {/* Primary CTA — Telegram Stars. */}
              <button
                onClick={handleStars}
                disabled={loading}
                className="w-full rounded-full bg-gradient-to-r from-amber-400 to-orange-500 px-6 py-3 text-sm font-bold text-black shadow-lg transition-transform active:scale-[0.97] disabled:opacity-60"
              >
                {loadingStars
                  ? t("premium.processing")
                  : t("premium.payStars", { price: starsPrice })}
              </button>

              {/* Secondary — Crypto Pay. Quieter outline so it doesn't
                  compete with the Stars CTA above. */}
              <button
                onClick={handleCrypto}
                disabled={loading}
                className="w-full rounded-full border border-input bg-transparent px-6 py-3 text-sm font-semibold text-foreground transition-colors hover:bg-accent active:scale-[0.97] disabled:opacity-60"
              >
                {loadingCrypto
                  ? t("premium.processing")
                  : t("premium.payCrypto", { price: cryptoPrice })}
              </button>

              <button
                onClick={onClose}
                className="w-full rounded-full px-6 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
              >
                {t("premium.later")}
              </button>
            </DrawerFooter>
          </>
        )}
      </DrawerContent>
    </Drawer>
  );
}
