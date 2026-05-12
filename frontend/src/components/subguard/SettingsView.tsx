import { useState } from "react";
import type { UserSettings } from "@/types/subscription";
import { Bell, Check, ChevronRight, Globe, Lock, Sparkles, Wallet } from "lucide-react";
import { toast } from "sonner";
import { SUPPORTED_CURRENCIES, currencySymbol } from "@/lib/currencyRates";
import { useSettingsStore } from "@/stores/settingsStore";
import { useTranslation } from "react-i18next";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import i18n from "@/lib/i18n";
import { hapticImpact, hapticSelection } from "@/lib/telegram";

interface Props {
  settings: UserSettings;
  user?: { name: string };
}

export function SettingsView({ settings, user }: Props) {
  const updateSettings = useSettingsStore((s) => s.updateSettings);
  const storeUser = useSettingsStore((s) => s.user);
  const { t } = useTranslation();

  const name = user?.name ?? storeUser?.name ?? "User";
  const photoUrl = storeUser?.photoUrl ?? "";
  const username = storeUser?.username ?? "";
  const initials = name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
  const isPremium = settings.isSubscribed;

  const [languageSheetOpen, setLanguageSheetOpen] = useState(false);

  // "Coming soon" handler — backend for these isn't ready yet, but the button
  // shouldn't feel broken. One info toast with localized copy.
  const comingSoon = () => {
    hapticImpact("light");
    toast.info(t("toast.comingSoon"));
  };

  const openLanguageSheet = () => {
    hapticImpact("light");
    setLanguageSheetOpen(true);
  };

  const pickLanguage = (locale: "ru" | "en") => {
    hapticSelection();
    void i18n.changeLanguage(locale);
    updateSettings({ locale });
    setLanguageSheetOpen(false);
  };

  // Admin panel deliberately omitted — management now lives natively inside
  // the Telegram bot, not in the client WebApp.
  const items: {
    Icon: typeof Bell;
    label: string;
    hint: string;
    onClick: () => void;
  }[] = [
    {
      Icon: Bell,
      label: t("settings.notifications"),
      hint: t("settings.notificationsHint"),
      onClick: comingSoon,
    },
    {
      Icon: Globe,
      label: t("settings.language"),
      hint: settings.locale === "ru" ? t("settings.languageRu") : t("settings.languageEn"),
      onClick: openLanguageSheet,
    },
    {
      Icon: Lock,
      label: t("settings.privacy"),
      hint: t("settings.privacyHint"),
      onClick: comingSoon,
    },
    {
      Icon: Sparkles,
      label: t("settings.pro"),
      hint: isPremium ? t("settings.proActive") : t("settings.proUpgrade"),
      onClick: comingSoon,
    },
  ];

  return (
    <div className="px-5">
      {/* Profile header */}
      <div className="bg-surface mb-5 flex items-center gap-4 rounded-2xl p-5">
        <Avatar className="h-16 w-16 shadow-elevated">
          <AvatarImage src={photoUrl || undefined} alt={name} />
          <AvatarFallback className="bg-gradient-primary text-xl font-bold text-white">
            {initials || "?"}
          </AvatarFallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <p className="truncate text-lg font-bold">{name}</p>
            <span
              className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${
                isPremium
                  ? "bg-gradient-primary text-white"
                  : "bg-surface-elevated text-muted-foreground"
              }`}
            >
              {isPremium ? t("settings.premium") : t("settings.freeUser")}
            </span>
          </div>
          {username && (
            <p className="mt-1 truncate text-xs text-muted-foreground">@{username}</p>
          )}
        </div>
      </div>

      {/* Base currency selector */}
      <div className="bg-surface mb-5 rounded-2xl p-4">
        <div className="mb-3 flex items-center gap-2">
          <Wallet className="h-4 w-4 text-muted-foreground" />
          <p className="text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
            {t("settings.baseCurrency")}
          </p>
        </div>
        <div className="flex gap-2">
          {SUPPORTED_CURRENCIES.map((c) => {
            const active = settings.defaultCurrency === c;
            return (
              <button
                key={c}
                onClick={() => updateSettings({ defaultCurrency: c })}
                className={`flex-1 rounded-xl py-2.5 text-center text-sm font-semibold transition-all ${
                  active
                    ? "bg-gradient-primary text-white shadow-elevated"
                    : "bg-surface-elevated text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="text-xs">{currencySymbol(c)}</span>{" "}
                {c}
              </button>
            );
          })}
        </div>
        <p className="mt-2 text-[11px] text-muted-foreground">
          {t("settings.currencyHint")}
        </p>
      </div>

      {/* Settings list */}
      <div className="space-y-2">
        {items.map(({ Icon, label, hint, onClick }) => (
          <button
            key={label}
            onClick={onClick}
            className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left transition-colors active:scale-[0.99]"
          >
            <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
              <Icon className="h-4 w-4" />
            </div>
            <div className="flex-1">
              <p className="text-sm font-semibold">{label}</p>
              {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
            </div>
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          </button>
        ))}
      </div>

      {/* Language picker sheet */}
      <Sheet open={languageSheetOpen} onOpenChange={setLanguageSheetOpen}>
        <SheetContent side="bottom" className="rounded-t-3xl">
          <SheetHeader className="text-left">
            <SheetTitle>{t("settings.languageSheetTitle")}</SheetTitle>
            <SheetDescription>{t("settings.languageSheetDesc")}</SheetDescription>
          </SheetHeader>
          <div className="mt-4 space-y-2 pb-6">
            {(
              [
                { code: "ru" as const, label: t("settings.languageRu") },
                { code: "en" as const, label: t("settings.languageEn") },
              ]
            ).map(({ code, label }) => {
              const active = settings.locale === code;
              return (
                <button
                  key={code}
                  onClick={() => pickLanguage(code)}
                  className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left transition-colors active:scale-[0.99]"
                >
                  <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                    <Globe className="h-4 w-4" />
                  </div>
                  <p className="flex-1 text-sm font-semibold">{label}</p>
                  {active && <Check className="h-4 w-4 text-primary" />}
                </button>
              );
            })}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
