import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Subscription } from "@/types/subscription";
import { localeFor } from "@/lib/format";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { SubscriptionCard } from "./SubscriptionCard";
import { ServiceLogo } from "./ServiceLogo";
import { COLOR_MAP, ICON_MAP } from "@/lib/customIcons";

/** Max number of full icons rendered per day cell before falling back to a
 *  "+N" overflow chip. Two keeps the row narrow inside the cell. */
const CALENDAR_ICON_LIMIT = 2;

/**
 * MiniSubIcon — single 16-px subscription avatar for the calendar cell.
 * Mirrors BrandIcon's branching at a smaller scale:
 *   - custom sub (Brand === "default") with a saved icon_name + icon_color
 *     → coloured circle + lucide glyph at 10 px
 *   - everything else (real brand / no custom assets) → ServiceLogo, which
 *     itself falls back to a gradient letter avatar
 * Both variants get a 2-px background-coloured ring so adjacent overlapped
 * icons read as separate tiles, the same trick AvatarGroup uses on the
 * shared-room cards.
 */
function MiniSubIcon({ sub }: { sub: Subscription }) {
  if (sub.brand === "default" && sub.icon_name && sub.icon_color) {
    const Icon = ICON_MAP[sub.icon_name];
    const colour = COLOR_MAP[sub.icon_color];
    if (Icon && colour) {
      return (
        <span
          aria-hidden="true"
          className={`flex h-4 w-4 items-center justify-center rounded-full ring-2 ring-background ${colour.bg}`}
        >
          <Icon size={10} strokeWidth={2.5} className="text-white" />
        </span>
      );
    }
  }
  return (
    <ServiceLogo
      brand={sub.brand}
      name={sub.name}
      size={16}
      rounded="full"
      className="ring-2 ring-background"
    />
  );
}

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
              return <div key={i} className="min-h-[3.25rem]" />;
            }
            const dateObj = new Date(year, month, d);
            const isToday = sameDay(dateObj, today);
            const isSelected = selectedDate && sameDay(dateObj, selectedDate);
            const dayBills = billsByDay.get(d) ?? [];
            return (
              <button
                key={i}
                onClick={() => setSelectedDate(isSelected ? null : dateObj)}
                className="relative flex min-h-[3.25rem] flex-col items-center pt-1 overflow-visible bg-transparent p-0"
              >
                <span
                  className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm transition-all ${
                    isSelected
                      ? "bg-gradient-primary font-bold text-white shadow-elevated"
                      : isToday
                        ? "bg-primary/15 text-primary font-semibold ring-1.5 ring-primary/40"
                        : dayBills.length > 0
                          ? "font-medium text-foreground hover:bg-surface-elevated"
                          : "text-muted-foreground hover:bg-surface-elevated"
                  }`}
                >
                  {d}
                </span>
                {dayBills.length > 0 && (
                  <div className="absolute -bottom-1 left-1/2 flex -translate-x-1/2 items-center justify-center">
                    {dayBills.slice(0, CALENDAR_ICON_LIMIT).map((s, idx) => (
                      <span
                        key={s.id}
                        className={idx > 0 ? "-ml-1" : ""}
                      >
                        <MiniSubIcon sub={s} />
                      </span>
                    ))}
                    {dayBills.length > CALENDAR_ICON_LIMIT && (
                      <span
                        aria-hidden="true"
                        className="bg-muted text-muted-foreground ring-2 ring-background -ml-1 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[8px] font-bold leading-none"
                      >
                        +{dayBills.length - CALENDAR_ICON_LIMIT}
                      </span>
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
