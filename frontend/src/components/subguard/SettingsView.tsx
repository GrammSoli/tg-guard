import type { UserSettings } from "@/types/subscription";
import { useNavigate } from "@tanstack/react-router";
import { Bell, ChevronRight, Globe, Lock, Shield, Sparkles, Wallet } from "lucide-react";
import { SUPPORTED_CURRENCIES, currencySymbol } from "@/lib/currencyRates";
import { useSettingsStore } from "@/stores/settingsStore";
import { useTranslation } from "react-i18next";

interface Props {
  settings: UserSettings;
  user?: { name: string };
}

export function SettingsView({ settings, user }: Props) {
  const { updateSettings } = useSettingsStore();
  const { t } = useTranslation();
  const name = user?.name ?? "Michael";
  const initials = name
    .split(" ")
    .map((p) => p[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
  const telegramId = "@michael_2847194";
  const isPremium = settings.isSubscribed;
  const isRu = settings.locale === "ru";

  const navigate = useNavigate();
  const items: {
    Icon: typeof Bell;
    label: string;
    hint: string;
    onClick?: () => void;
  }[] = [
    { Icon: Bell, label: t("settings.notifications"), hint: t("settings.notificationsHint") },
    { Icon: Globe, label: t("settings.language"), hint: t("settings.languageHint") },
    { Icon: Lock, label: t("settings.privacy"), hint: t("settings.privacyHint") },
    { Icon: Sparkles, label: t("settings.pro"), hint: isPremium ? t("settings.proActive") : t("settings.proUpgrade") },
    ...(settings.isAdmin
      ? [{
          Icon: Shield,
          label: t("settings.admin"),
          hint: t("settings.adminHint"),
          onClick: () => navigate({ to: "/admin" }),
        }]
      : []),
  ];

  return (
    <div className="px-5">
      {/* Profile header */}
      <div className="bg-surface mb-5 flex items-center gap-4 rounded-2xl p-5">
        <div className="bg-gradient-primary flex h-16 w-16 items-center justify-center rounded-full text-xl font-bold text-white shadow-elevated">
          {initials}
        </div>
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
          <p className="mt-1 truncate text-xs text-muted-foreground">{telegramId}</p>
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
    </div>
  );
}
