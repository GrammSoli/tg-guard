import { useTranslation } from "react-i18next";
import { Search, SlidersHorizontal, X } from "lucide-react";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { hapticImpact } from "@/lib/telegram";

interface Props {
  value: string;
  onChange: (v: string) => void;
  onOpenFilters: () => void;
}

/**
 * FilterBar — search input + filter-sheet trigger. The trigger gets a small
 * gradient dot in its top-right corner when any non-default filter is
 * active (sortBy != nextPayment, or any type / category selection), so the
 * user has a visual cue that the list they're looking at is filtered.
 */
export function FilterBar({ value, onChange, onOpenFilters }: Props) {
  const { t } = useTranslation();
  const hasActiveFilters = useSubscriptionStore((s) => s.hasActiveFilters)();

  return (
    <div className="px-5">
      <div className="bg-surface-elevated flex items-center gap-3 rounded-2xl px-4 py-3">
        <Search className="h-4 w-4 text-muted-foreground" />
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={t("filter.placeholder")}
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        />
        {value && (
          <button
            onClick={() => onChange("")}
            className="flex h-5 w-5 items-center justify-center rounded-full bg-muted-foreground/20 text-muted-foreground transition-colors hover:bg-muted-foreground/30"
            aria-label={t("filter.clear")}
          >
            <X className="h-3 w-3" />
          </button>
        )}
        <button
          type="button"
          onClick={() => {
            hapticImpact("light");
            onOpenFilters();
          }}
          className="relative flex h-7 w-7 items-center justify-center rounded-lg transition-colors active:scale-95"
          aria-label={t("filter.openFilters")}
        >
          <SlidersHorizontal
            className={`h-4 w-4 transition-colors ${
              hasActiveFilters ? "text-primary" : "text-muted-foreground"
            }`}
          />
          {hasActiveFilters && (
            <span
              className="bg-gradient-primary absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full"
              aria-hidden="true"
            />
          )}
        </button>
      </div>
    </div>
  );
}
