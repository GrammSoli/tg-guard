import { formatCurrency, localeFor } from "@/lib/format";
import { useTranslation } from "react-i18next";
import { convertCurrency } from "@/lib/currencyRates";
import { ArrowUpRight } from "lucide-react";
import type { UserSettings } from "@/types/subscription";

interface Props {
  activeCount: number;
  totalMonthly: number;
  currency: string;
  user?: { name: string };
  settings: UserSettings;
}

export function SummaryHeader({
  activeCount,
  totalMonthly,
  currency,
  user,
  settings,
}: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const lc = localeFor(locale);
  const baseCurrency = settings.defaultCurrency;
  const displayTotal =
    currency !== baseCurrency
      ? convertCurrency(totalMonthly, currency, baseCurrency)
      : totalMonthly;

  return (
    <header className="safe-top relative px-5 pb-6 pt-4">
      <div className="mb-5">
        <p className="text-xs text-muted-foreground">{t("welcome.greeting")}</p>
        <p className="text-base font-semibold">{user?.name ?? "Alex"}</p>
      </div>

      <div
        className="relative overflow-hidden rounded-[2rem] bg-gradient-to-br from-purple-800 via-violet-600 to-fuchsia-600 p-6 shadow-xl shadow-purple-500/20"
      >
        <div
          aria-hidden
          className="pointer-events-none absolute -right-10 -top-10 h-44 w-44 rounded-full bg-white/15 blur-2xl"
        />
        <div
          aria-hidden
          className="pointer-events-none absolute -bottom-16 -left-10 h-44 w-44 rounded-full bg-fuchsia-300/20 blur-3xl"
        />

        <p className="text-sm text-white/70">
          {t("header.totalMonthlySpend")}
        </p>
        <p className="mt-2 text-5xl font-extrabold tracking-tight text-white">
          {formatCurrency(displayTotal, baseCurrency, lc)}
        </p>
        <div className="mt-4 inline-flex items-center gap-1 rounded-full bg-white/20 px-3 py-1 text-xs font-medium text-white backdrop-blur-sm">
          <ArrowUpRight className="h-3 w-3" />
          {baseCurrency}
        </div>

        <p className="mt-5 text-[10px] font-semibold uppercase tracking-[0.18em] text-white/60">
          {t("header.totalCostPrefix")} {activeCount}{" "}
          {t("header.activeSubscriptions")}
        </p>
      </div>
    </header>
  );
}
