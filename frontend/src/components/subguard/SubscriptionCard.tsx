import { memo } from "react";
import { formatCurrency, formatDate, localeFor } from "@/lib/format";
import { convertCurrency, useFxRates } from "@/lib/currencyRates";
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

// Wrapped in React.memo because this card lives inside a .map() of 20-100
// items on the dashboard. Without memoization, every Dashboard re-render
// (search keystroke, filter change, any store update) re-renders every
// card. The props (subscription object + onClick callback) are stable
// references from the parent's useMemo / useCallback path, so shallow
// compare correctly short-circuits when nothing changed. See audit F5.
export const SubscriptionCard = memo(function SubscriptionCard({
  subscription,
  onClick,
}: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const s = subscription;
  const lc = localeFor(locale);
  // Granular selector — was destructuring the whole settings object and
  // re-rendering on every settings change ×N cards in the list. The
  // worst case in the project: any toggle/save in NotificationsSheet
  // would re-render 100 cards. See audit F2.
  const userCurrency = useSettingsStore((s) => s.settings.defaultCurrency);
  // Subscribe to FX rates so the card re-renders when /api/v1/fx lands
  // a fresh snapshot. Value not used directly — convertCurrency reads
  // via the same store getter — but the subscription is what triggers
  // the rerender.
  useFxRates();

  // Convert to user's preferred currency
  const displayAmount = convertCurrency(s.amount, s.currency, userCurrency);

  // Overdue = next_payment_at is strictly before today's midnight (i.e.
  // it was due yesterday or earlier). Trials don't get the overdue badge —
  // an expired trial is a separate UX concern.
  const todayMidnight = new Date();
  todayMidnight.setHours(0, 0, 0, 0);
  const isOverdue =
    !s.is_trial && new Date(s.next_payment_at).getTime() < todayMidnight.getTime();

  let dateLabel: string;
  if (s.is_trial && s.trial_ends_at) {
    dateLabel = `${t("card.trialEndsOn")} ${formatDate(s.trial_ends_at, lc)}`;
  } else if (isOverdue) {
    dateLabel = `${t("card.overdue")} (${formatDate(s.next_payment_at, lc)})`;
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
      <BrandIcon brand={s.brand} iconName={s.icon_name} iconColor={s.icon_color} />
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
        <p
          className={`mt-0.5 truncate text-xs ${
            isOverdue ? "font-semibold text-destructive" : "text-muted-foreground"
          }`}
        >
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
});
