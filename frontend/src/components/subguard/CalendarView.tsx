import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Subscription } from "@/types/subscription";
import { localeFor } from "@/lib/format";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { SubscriptionCard } from "./SubscriptionCard";
import { brandColorFor } from "@/lib/brandLogo";

interface Props {
  subscriptions: Subscription[];
  onEdit?: (s: Subscription) => void;
}

const sameDay = (a: Date, b: Date) =>
  a.getFullYear() === b.getFullYear() &&
  a.getMonth() === b.getMonth() &&
  a.getDate() === b.getDate();

export function CalendarView({ subscriptions, onEdit }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const lc = localeFor(locale);
  const today = new Date();

  const [cursor, setCursor] = useState(new Date(today.getFullYear(), today.getMonth(), 1));
  const [selectedDate, setSelectedDate] = useState<Date | null>(null);

  const year = cursor.getFullYear();
  const month = cursor.getMonth();

  const startOffset = (new Date(year, month, 1).getDay() + 6) % 7;
  const daysInMonth = new Date(year, month + 1, 0).getDate();

  const billsByDay = useMemo(() => {
    const map = new Map<number, Subscription[]>();
    for (const s of subscriptions) {
      const d = new Date(s.next_payment_at);
      if (d.getMonth() === month && d.getFullYear() === year) {
        const day = d.getDate();
        if (!map.has(day)) map.set(day, []);
        map.get(day)!.push(s);
      }
    }
    return map;
  }, [subscriptions, month, year]);

  const cells: (number | null)[] = [];
  for (let i = 0; i < startOffset; i++) cells.push(null);
  for (let d = 1; d <= daysInMonth; d++) cells.push(d);

  const monthLabel = new Intl.DateTimeFormat(lc, {
    month: "long",
    year: "numeric",
  }).format(cursor);

  const list = useMemo(() => {
    if (selectedDate) {
      return subscriptions.filter((s) =>
        sameDay(new Date(s.next_payment_at), selectedDate),
      );
    }
    return [...subscriptions]
      .filter((s) => {
        const d = new Date(s.next_payment_at);
        return d.getMonth() === month && d.getFullYear() === year;
      })
      .sort((a, b) => +new Date(a.next_payment_at) - +new Date(b.next_payment_at));
  }, [subscriptions, selectedDate, month, year]);

  const shiftMonth = (delta: number) => {
    setSelectedDate(null);
    setCursor(new Date(year, month + delta, 1));
  };

  return (
    <div className="px-5">
      <div className="bg-surface rounded-2xl p-4">
        <div className="mb-3 flex items-center justify-between">
          <button
            onClick={() => shiftMonth(-1)}
            className="bg-surface-elevated flex h-8 w-8 items-center justify-center rounded-full transition-transform active:scale-90"
            aria-label="Previous month"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <p className="text-base font-semibold capitalize">{monthLabel}</p>
          <button
            onClick={() => shiftMonth(1)}
            className="bg-surface-elevated flex h-8 w-8 items-center justify-center rounded-full transition-transform active:scale-90"
            aria-label="Next month"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>

        <div className="grid grid-cols-7 gap-1 text-center text-[10px] font-semibold uppercase text-muted-foreground">
          {(["mo", "tu", "we", "th", "fr", "sa", "su"] as const).map((d) => (
            <div key={d} className="py-1">
              {t(`calendar.${d}`)}
            </div>
          ))}
        </div>

        <div className="grid grid-cols-7">
          {cells.map((d, i) => {
            if (d === null) {
              return <div key={i} className="aspect-square" />;
            }
            const dateObj = new Date(year, month, d);
            const isToday = sameDay(dateObj, today);
            const isSelected = selectedDate && sameDay(dateObj, selectedDate);
            const dayBills = billsByDay.get(d) ?? [];
            return (
              <button
                key={i}
                onClick={() => setSelectedDate(isSelected ? null : dateObj)}
                className="relative flex aspect-square flex-col items-center justify-center gap-1 rounded-xl transition-colors hover:bg-surface-elevated"
              >
                <span
                  className={`flex h-8 w-8 items-center justify-center rounded-full text-sm transition-all ${
                    isSelected
                      ? "bg-gradient-primary font-bold text-white shadow-elevated"
                      : isToday
                        ? "bg-primary/15 text-primary font-semibold ring-1.5 ring-primary/40"
                        : dayBills.length > 0
                          ? "font-medium text-foreground"
                          : "text-muted-foreground"
                  }`}
                >
                  {d}
                </span>
                {dayBills.length > 0 && (
                  <div className="flex items-center justify-center gap-0.5">
                    {dayBills.slice(0, 3).map((s) => (
                      <span
                        key={s.id}
                        className="inline-block h-1.5 w-1.5 rounded-full"
                        style={{ backgroundColor: brandColorFor(s.brand) }}
                      />
                    ))}
                    {dayBills.length > 3 && (
                      <span className="inline-block h-1.5 w-1.5 rounded-full bg-muted-foreground/50" />
                    )}
                  </div>
                )}
              </button>
            );
          })}
        </div>
      </div>

      <p className="mt-6 mb-3 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("calendar.comingUp")}
      </p>
      <div className="space-y-2">
        {list.map((s) => (
          <SubscriptionCard key={s.id} subscription={s} onClick={onEdit} />
        ))}
        {list.length === 0 && (
          <p className="py-6 text-center text-sm text-muted-foreground">
            {t("calendar.nothingScheduled")}
          </p>
        )}
      </div>
    </div>
  );
}
