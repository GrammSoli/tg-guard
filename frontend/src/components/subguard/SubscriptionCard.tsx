import { formatCurrency, formatDate, localeFor } from "@/lib/format";
import { convertCurrency } from "@/lib/currencyRates";
import { useTranslation } from "react-i18next";
import type { Subscription } from "@/types/subscription";
import { useSettingsStore } from "@/stores/settingsStore";
import { Clock3 } from "lucide-react";
import { BrandIcon } from "./BrandIcon";
import { DateTz } from "./DateTz";

interface Props {
  subscription: Subscription;
  onClick?: (s: Subscription) => void;
}

export function SubscriptionCard({ subscription, onClick }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const s = subscription;
  const lc = localeFor(locale);
  const { settings } = useSettingsStore();
  const userCurrency = settings.defaultCurrency;

  // Convert to user's preferred currency
  const displayAmount = convertCurrency(s.amount, s.currency, userCurrency);

  let dateLabel: string;
  if (s.is_trial && s.trial_ends_at) {
    dateLabel = `${t("card.trialEndsOn")} ${formatDate(s.trial_ends_at, lc)}`;
  } else if (!s.is_auto_pay) {
    dateLabel = `${t("card.manualPayment")} ${formatDate(s.next_payment_at, lc)}`;
  } else {
    dateLabel = `${t("card.renewsOn")} ${formatDate(s.next_payment_at, lc)}`;
  }

  return (
    <button
      onClick={() => onClick?.(s)}
      className="bg-surface group flex w-full items-center gap-3 rounded-2xl p-3 text-left transition-transform active:scale-[0.98]"
    >
      <BrandIcon brand={s.brand} />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <p className="truncate text-[15px] font-semibold">
            {s.name}
            {s.note && (
              <span className="ml-1 font-normal text-muted-foreground">
                · {s.note}
              </span>
            )}
          </p>
          {s.is_trial && (
            <span className="bg-trial text-trial-foreground rounded-full px-1.5 py-0.5 text-[9px] font-bold tracking-wide">
              {t("card.trial")}
            </span>
          )}
          {!s.is_trial && s.is_auto_pay && (
            <span className="bg-primary/15 text-primary rounded-full px-1.5 py-0.5 text-[9px] font-bold tracking-wide">
              {t("card.autoPay")}
            </span>
          )}
        </div>
        <p className="mt-0.5 truncate text-xs text-muted-foreground">
          <DateTz>{dateLabel}</DateTz>
        </p>
      </div>
      <div className="flex items-center gap-2">
        {!s.is_auto_pay && !s.is_trial && (
          <Clock3 className="h-4 w-4 text-muted-foreground" />
        )}
        <p className="text-base font-bold tabular-nums">
          {formatCurrency(displayAmount, userCurrency, lc)}
        </p>
      </div>
    </button>
  );
}
