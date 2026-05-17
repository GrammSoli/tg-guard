import { useMemo, useState } from "react";
import { useDragScroll } from "@/hooks/useDragScroll";
import { subDays, subMonths, startOfMonth, endOfMonth, isWithinInterval, startOfDay, endOfDay, format } from "date-fns";
import { ru as ruLocale } from "date-fns/locale/ru";
import { CalendarIcon } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { Subscription } from "@/types/subscription";
import { formatCurrency, formatDate, localeFor } from "@/lib/format";
import { convertCurrency, useFxRates } from "@/lib/currencyRates";
import { periodToMonthly } from "@/lib/subscriptionMath";
import { BrandIcon } from "./BrandIcon";
import { domainHintFromName } from "@/lib/brandfetch";
import { DateTz } from "./DateTz";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";
import type { DateRange } from "react-day-picker";

interface Props {
  subscriptions: Subscription[];
  currency: string;
}

type PresetKey = "7d" | "30d" | "this_month" | "3m" | "all" | "custom";

interface Preset {
  key: PresetKey;
  labelKey: string;
  range: () => { from: Date; to: Date };
}

const presets: Preset[] = [
  { key: "7d", labelKey: "analytics.7d", range: () => ({ from: subDays(new Date(), 7), to: new Date() }) },
  { key: "30d", labelKey: "analytics.30d", range: () => ({ from: subDays(new Date(), 30), to: new Date() }) },
  { key: "this_month", labelKey: "analytics.thisMonth", range: () => ({ from: startOfMonth(new Date()), to: endOfMonth(new Date()) }) },
  { key: "3m", labelKey: "analytics.3m", range: () => ({ from: subMonths(new Date(), 3), to: new Date() }) },
  // "all" — open-ended range relative to today rather than a hardcoded
  // 2020-2030 window. Anchored at year 2000 (predates the project) and
  // today+5y so a far-future trial_ends_at still falls inside. Audit Low.
  { key: "all", labelKey: "analytics.allTime", range: () => {
    const now = new Date();
    return { from: new Date(2000, 0, 1), to: new Date(now.getFullYear() + 5, 11, 31) };
  } },
];

const toBase = (s: Subscription, baseCurrency: string) =>
  convertCurrency(periodToMonthly(s), s.currency, baseCurrency);

export function AnalyticsView({ subscriptions, currency }: Props) {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const lc = localeFor(locale);

  // Subscribe to FX rates so all the conversions below recompute when
  // /api/v1/fx lands. Value not used directly — convertCurrency reads
  // the same store getter — but the subscription drives the rerender.
  useFxRates();

  const [activePreset, setActivePreset] = useState<PresetKey>("all");
  const [dateRange, setDateRange] = useState<{ from: Date; to: Date }>(
    presets.find((p) => p.key === "all")!.range(),
  );
  const [calendarOpen, setCalendarOpen] = useState(false);
  const dragScrollRef = useDragScroll<HTMLDivElement>();

  const handlePreset = (p: Preset) => {
    setActivePreset(p.key);
    setDateRange(p.range());
  };

  const handleCustomRange = (range: DateRange | undefined) => {
    if (range?.from) {
      setActivePreset("custom");
      setDateRange({
        from: startOfDay(range.from),
        to: range.to ? endOfDay(range.to) : endOfDay(range.from),
      });
    }
  };

  const filtered = useMemo(() => {
    return subscriptions.filter((s) => {
      const d = new Date(s.next_payment_at);
      return isWithinInterval(d, { start: startOfDay(dateRange.from), end: endOfDay(dateRange.to) });
    });
  }, [subscriptions, dateRange]);

  const monthly = filtered.reduce((sum, x) => sum + toBase(x, currency), 0);
  const yearly = monthly * 12;
  const trials = filtered.filter((s) => s.is_trial).length;
  const manual = filtered.filter((s) => !s.is_auto_pay).length;

  const byCategory = new Map<string, number>();
  for (const s of filtered) {
    const k = s.tag ?? "Other";
    byCategory.set(k, (byCategory.get(k) ?? 0) + toBase(s, currency));
  }
  const categories = [...byCategory.entries()].sort((a, b) => b[1] - a[1]);
  const max = Math.max(...categories.map(([, v]) => v), 1);

  const upcoming = [...filtered]
    .sort((a, b) => new Date(a.next_payment_at).getTime() - new Date(b.next_payment_at).getTime())
    .slice(0, 4);

  const dateFnsLocale = locale === "ru" ? ruLocale : undefined;

  const rangeLabel =
    activePreset === "custom"
      ? `${format(dateRange.from, "d MMM", { locale: dateFnsLocale })} – ${format(dateRange.to, "d MMM", { locale: dateFnsLocale })}`
      : activePreset === "all"
        ? t("analytics.allTime")
        : presets.find((p) => p.key === activePreset) ? t(presets.find((p) => p.key === activePreset)!.labelKey) : "";

  return (
    <div className="space-y-5 px-5">
      {/* ── Date Range Selector ── */}
      <div className="flex items-center gap-2">
        <div
          ref={dragScrollRef}
          className="no-scrollbar flex flex-1 gap-1.5 overflow-x-auto select-none cursor-grab active:cursor-grabbing"
        >
          {presets.map((p) => (
            <button
              key={p.key}
              onClick={() => handlePreset(p)}
              className={cn(
                "shrink-0 rounded-full px-3 py-1.5 text-[11px] font-semibold transition-colors",
                activePreset === p.key
                  ? "bg-gradient-primary text-white shadow-elevated"
                  : "bg-surface text-muted-foreground hover:bg-surface-elevated",
              )}
            >
              {t(p.labelKey)}
            </button>
          ))}
        </div>

        <Popover open={calendarOpen} onOpenChange={setCalendarOpen}>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              className={cn(
                "h-7 w-7 shrink-0 rounded-full p-0",
                activePreset === "custom"
                  ? "ring-1.5 ring-primary/50 text-primary"
                  : "text-muted-foreground hover:bg-surface-elevated",
              )}
            >
              <CalendarIcon className="h-3.5 w-3.5" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-auto p-0" align="end">
            <Calendar
              mode="range"
              selected={{ from: dateRange.from, to: dateRange.to }}
              onSelect={handleCustomRange}
              numberOfMonths={1}
              locale={dateFnsLocale}
              className={cn("p-3 pointer-events-auto")}
              disabled={(date) => date > new Date()}
            />
          </PopoverContent>
        </Popover>
      </div>

      {/* ── Period label ── */}
      <p className="text-center text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {rangeLabel}
      </p>

      {/* ── KPI Cards ── */}
      <div className="grid grid-cols-2 gap-3">
        <Stat label={t("analytics.monthly")} value={formatCurrency(monthly, currency, lc)} />
        <Stat label={t("analytics.yearly")} value={formatCurrency(yearly, currency, lc)} />
        <Stat label={t("analytics.active")} value={String(filtered.length)} />
        <Stat label={t("analytics.trialsManual")} value={`${trials} / ${manual}`} />
      </div>

      {/* ── Spend by Category ── */}
      <section className="bg-surface rounded-2xl p-4">
        <p className="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("analytics.spendByCategory")}
        </p>
        {categories.length === 0 ? (
          <p className="py-4 text-center text-xs text-muted-foreground">
            {t("analytics.noData")}
          </p>
        ) : (
          <div className="space-y-3">
            {categories.map(([name, val]) => (
              <div key={name}>
                <div className="mb-1 flex justify-between text-xs">
                  <span className="font-medium">{t(`category.${name.toLowerCase()}`, name)}</span>
                  <span className="tabular-nums text-muted-foreground">
                    {formatCurrency(val, currency, lc)}
                  </span>
                </div>
                <div className="bg-surface-elevated h-2 overflow-hidden rounded-full">
                  <div
                    className="bg-gradient-primary h-full rounded-full transition-all duration-300"
                    style={{ width: `${(val / max) * 100}%` }}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {/* ── Upcoming ── */}
      <section>
        <p className="mb-3 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
          {t("analytics.upcoming")}
        </p>
        <div className="space-y-2">
          {upcoming.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">
              {t("analytics.noData")}
            </p>
          ) : (
            upcoming.map((s) => (
              <div
                key={s.id}
                className="bg-surface flex items-center gap-3 rounded-2xl p-3"
              >
                <BrandIcon
                  brand={s.brand}
                  size="sm"
                  iconName={s.icon_name}
                  iconColor={s.icon_color}
                  domain={domainHintFromName(s.brand, s.name)}
                />
                <div className="flex-1">
                  <p className="text-sm font-semibold">{s.name}</p>
                  <p className="text-xs text-muted-foreground">
                    <DateTz>{formatDate(s.next_payment_at, lc)}</DateTz>
                  </p>
                </div>
                <p className="text-sm font-bold tabular-nums">
                  {formatCurrency(convertCurrency(s.amount, s.currency, currency), currency, lc)}
                </p>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-surface rounded-2xl p-4">
      <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </p>
      <p className="mt-1 text-xl font-bold tabular-nums">{value}</p>
    </div>
  );
}
