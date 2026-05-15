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

type Plan = "month" | "lifetime";

/**
 * Premium upgrade bottom-sheet. Two states:
 *
 *  - Already a donator → confirmation screen: crown, thank-you copy,
 *    the grant's expiry ("Active until …" or "Lifetime access"), a
 *    single Close button.
 *  - Free user → upgrade screen: feature cards, a Month/Lifetime plan
 *    selector (Lifetime pre-selected, badged "Best value"), and two
 *    payment buttons. Stars is the primary CTA; Crypto the quiet
 *    secondary. Both POST the chosen plan to the backend.
 *
 * Pricing is plan-split and admin-configurable, sourced from the
 * paywall config (GET /api/v1/config). Stars prices are locale-split;
 * crypto is a single USD amount per plan.
 */
export function PremiumSheet({ open, onClose }: PremiumSheetProps) {
  const { t, i18n } = useTranslation();
  const config = usePaywallStore((s) => s.config);
  const fetchProfile = useSettingsStore((s) => s.fetchProfile);
  // is_donator + premium_expires_at from /me.
  const isDonator = useSettingsStore((s) => s.settings.isSubscribed);
  const premiumExpiresAt = useSettingsStore((s) => s.settings.premiumExpiresAt);
  const [selectedPlan, setSelectedPlan] = useState<Plan>("lifetime");
  const [loadingStars, setLoadingStars] = useState(false);
  const [loadingCrypto, setLoadingCrypto] = useState(false);

  const isRu = i18n.language.startsWith("ru");
  const starsMonth = isRu ? config.price_stars_month_ru : config.price_stars_month_en;
  const starsLifetime = isRu ? config.price_stars_lifetime_ru : config.price_stars_lifetime_en;
  const cryptoMonth = isRu ? config.price_crypto_month_usd_ru : config.price_crypto_month_usd_en;
  const cryptoLifetime = isRu
    ? config.price_crypto_lifetime_usd_ru
    : config.price_crypto_lifetime_usd_en;

  const starsPrice = selectedPlan === "month" ? starsMonth : starsLifetime;
  const cryptoPrice = selectedPlan === "month" ? cryptoMonth : cryptoLifetime;

  // ── Stars payment flow ────────────────────────────────────
  const handleStars = async () => {
    hapticImpact("medium");
    setLoadingStars(true);
    try {
      const { invoice_url } = await api<{ invoice_url: string }>(
        "/payments/stars",
        { method: "POST", body: { plan: selectedPlan } },
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
        { method: "POST", body: { plan: selectedPlan } },
      );
      // Open the CryptoBot link inside Telegram. The webhook activates
      // premium on the backend; the user gets a TG message.
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

  // ── Already-active state ────────────────────────────────────
  if (isDonator) {
    let statusLine: string;
    if (premiumExpiresAt) {
      const date = new Date(premiumExpiresAt).toLocaleDateString(
        isRu ? "ru-RU" : "en-US",
        { day: "numeric", month: "long", year: "numeric" },
      );
      statusLine = t("premium.activeUntil", { date });
    } else {
      statusLine = t("premium.lifetimeAccess");
    }
    return (
      <Drawer open={open} onOpenChange={(v) => !v && onClose()}>
        <DrawerContent>
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

          <div className="px-6">
            <div className="rounded-xl bg-surface p-3 text-center text-sm font-semibold text-amber-500">
              {statusLine}
            </div>
          </div>

          <DrawerFooter>
            <button
              onClick={onClose}
              className="w-full rounded-full bg-primary px-6 py-3 text-sm font-bold text-primary-foreground shadow-lg transition-transform active:scale-[0.97]"
            >
              {t("premium.close")}
            </button>
          </DrawerFooter>
        </DrawerContent>
      </Drawer>
    );
  }

  // ── Upgrade state ───────────────────────────────────────────
  return (
    <Drawer open={open} onOpenChange={(v) => !v && onClose()}>
      <DrawerContent>
        <DrawerHeader className="text-center">
          <div className="mx-auto mb-3 flex h-16 w-16 items-center justify-center rounded-full bg-gradient-to-br from-amber-400/20 to-orange-500/20">
            <Crown className="h-8 w-8 text-amber-400" />
          </div>
          <DrawerTitle className="text-xl">{t("premium.title")}</DrawerTitle>
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

        {/* Plan selector — Month vs Lifetime, side by side. */}
        <div className="grid grid-cols-2 gap-3 px-6 pt-3">
          <PlanCard
            label={t("premium.planMonth")}
            stars={starsMonth}
            usd={cryptoMonth}
            selected={selectedPlan === "month"}
            onSelect={() => setSelectedPlan("month")}
          />
          <PlanCard
            label={t("premium.planLifetime")}
            stars={starsLifetime}
            usd={cryptoLifetime}
            badge={t("premium.bestValue")}
            selected={selectedPlan === "lifetime"}
            onSelect={() => setSelectedPlan("lifetime")}
          />
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

          {/* Secondary — Crypto Pay. */}
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
      </DrawerContent>
    </Drawer>
  );
}

interface PlanCardProps {
  label: string;
  stars: number;
  usd: number;
  badge?: string;
  selected: boolean;
  onSelect: () => void;
}

/** A single tariff card in the Month/Lifetime selector. */
function PlanCard({ label, stars, usd, badge, selected, onSelect }: PlanCardProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`relative rounded-xl border p-3 text-center transition-colors ${
        selected
          ? "border-amber-400 bg-amber-400/10 ring-1 ring-amber-400/40"
          : "border-input bg-surface hover:bg-accent"
      }`}
    >
      {badge && (
        <span className="absolute -top-2 left-1/2 -translate-x-1/2 whitespace-nowrap rounded-full bg-amber-400 px-2 py-0.5 text-[10px] font-bold text-black">
          {badge}
        </span>
      )}
      <p className="text-sm font-bold">{label}</p>
      <p className="mt-1 text-sm font-semibold">⭐ {stars}</p>
      <p className="text-xs text-muted-foreground">${usd}</p>
    </button>
  );
}
