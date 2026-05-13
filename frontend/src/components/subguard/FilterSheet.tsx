import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Check } from "lucide-react";
import {
  useSubscriptionStore,
  DEFAULT_SORT,
  type SortBy,
  type FilterType,
} from "@/stores/subscriptionStore";
import { SERVICE_CATEGORIES, type ServiceCategory } from "@/lib/mockData";
import { categoryKey } from "@/lib/categoryKey";
import { hapticImpact, hapticSelection } from "@/lib/telegram";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const SORT_OPTIONS: SortBy[] = [
  "nextPayment",
  "priceDesc",
  "priceAsc",
  "alphabetical",
];

const TYPE_OPTIONS: FilterType[] = ["subscription", "room"];

/**
 * FilterSheet — advanced sort + filter controls for the dashboard list.
 *
 * State strategy: we keep a LOCAL draft of the filter values while the sheet
 * is open and only commit on "Apply". That lets the user fiddle with
 * options without yanking the list around in the background, and gives a
 * clean "Reset" button that doesn't immediately empty the screen.
 */
export function FilterSheet({ open, onOpenChange }: Props) {
  const { t } = useTranslation();

  const sortBy = useSubscriptionStore((s) => s.sortBy);
  const filterTypes = useSubscriptionStore((s) => s.filterTypes);
  const filterCategories = useSubscriptionStore((s) => s.filterCategories);
  const setSortBy = useSubscriptionStore((s) => s.setSortBy);
  const setFilterTypes = useSubscriptionStore((s) => s.setFilterTypes);
  const setFilterCategories = useSubscriptionStore((s) => s.setFilterCategories);
  const resetFilters = useSubscriptionStore((s) => s.resetFilters);

  // Draft state, rehydrated from the store every time the sheet (re)opens.
  const [draftSort, setDraftSort] = useState<SortBy>(sortBy);
  const [draftTypes, setDraftTypes] = useState<FilterType[]>(filterTypes);
  const [draftCategories, setDraftCategories] =
    useState<ServiceCategory[]>(filterCategories);

  useEffect(() => {
    if (!open) return;
    setDraftSort(sortBy);
    setDraftTypes(filterTypes);
    setDraftCategories(filterCategories);
  }, [open, sortBy, filterTypes, filterCategories]);

  const toggleType = (type: FilterType, checked: boolean) => {
    hapticSelection();
    setDraftTypes((prev) =>
      checked ? [...prev, type] : prev.filter((t) => t !== type),
    );
  };

  const toggleCategory = (cat: ServiceCategory) => {
    hapticSelection();
    setDraftCategories((prev) =>
      prev.includes(cat) ? prev.filter((c) => c !== cat) : [...prev, cat],
    );
  };

  const handleReset = () => {
    hapticImpact("light");
    setDraftSort(DEFAULT_SORT);
    setDraftTypes([]);
    setDraftCategories([]);
    resetFilters();
  };

  const handleApply = () => {
    hapticImpact("medium");
    setSortBy(draftSort);
    setFilterTypes(draftTypes);
    setFilterCategories(draftCategories);
    onOpenChange(false);
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="bottom"
        className="max-h-[85vh] overflow-y-auto rounded-t-3xl"
      >
        <SheetHeader className="text-left">
          <SheetTitle>{t("filter.sheetTitle")}</SheetTitle>
          <SheetDescription>{t("filter.sheetDesc")}</SheetDescription>
        </SheetHeader>

        <div className="mt-4 space-y-6 pb-2">
          {/* ── Sort ─────────────────────────────────────── */}
          <section>
            <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
              {t("filter.sortHeader")}
            </p>
            <RadioGroup
              value={draftSort}
              onValueChange={(v) => {
                hapticSelection();
                setDraftSort(v as SortBy);
              }}
              className="space-y-1.5"
            >
              {SORT_OPTIONS.map((option) => (
                <Label
                  key={option}
                  htmlFor={`sort-${option}`}
                  className="bg-surface flex w-full cursor-pointer items-center gap-3 rounded-2xl p-3"
                >
                  <RadioGroupItem id={`sort-${option}`} value={option} />
                  <span className="flex-1 text-sm font-medium">
                    {t(`filter.sort.${option}`)}
                  </span>
                </Label>
              ))}
            </RadioGroup>
          </section>

          {/* ── Type ─────────────────────────────────────── */}
          <section>
            <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
              {t("filter.typeHeader")}
            </p>
            <p className="mb-2 px-1 text-[11px] text-muted-foreground">
              {t("filter.typeHint")}
            </p>
            <div className="space-y-1.5">
              {TYPE_OPTIONS.map((type) => {
                const checked = draftTypes.includes(type);
                return (
                  <Label
                    key={type}
                    htmlFor={`type-${type}`}
                    className="bg-surface flex w-full cursor-pointer items-center gap-3 rounded-2xl p-3"
                  >
                    <Checkbox
                      id={`type-${type}`}
                      checked={checked}
                      onCheckedChange={(c) => toggleType(type, c === true)}
                    />
                    <span className="flex-1 text-sm font-medium">
                      {t(`filter.type.${type}`)}
                    </span>
                  </Label>
                );
              })}
            </div>
          </section>

          {/* ── Categories ───────────────────────────────── */}
          <section>
            <p className="mb-2 px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
              {t("filter.categoriesHeader")}
            </p>
            <div className="flex flex-wrap gap-2">
              {SERVICE_CATEGORIES.map((cat) => {
                const active = draftCategories.includes(cat);
                return (
                  <button
                    key={cat}
                    type="button"
                    onClick={() => toggleCategory(cat)}
                    className={`inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-xs font-semibold transition-colors ${
                      active
                        ? "bg-gradient-primary text-white shadow-elevated"
                        : "bg-surface text-muted-foreground hover:text-foreground"
                    }`}
                  >
                    {active && <Check className="h-3 w-3" />}
                    {t(categoryKey(cat))}
                  </button>
                );
              })}
            </div>
          </section>
        </div>

        {/* ── Actions ──────────────────────────────────── */}
        <div className="sticky bottom-0 -mx-6 mt-4 grid grid-cols-2 gap-2 border-t border-border bg-background px-6 py-3">
          <Button variant="outline" onClick={handleReset}>
            {t("filter.reset")}
          </Button>
          <Button onClick={handleApply} className="bg-gradient-primary text-white">
            {t("filter.apply")}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}
