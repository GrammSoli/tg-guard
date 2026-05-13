import {
  POPULAR_SERVICES,
  SERVICE_CATEGORIES,
  type PopularService,
  type ServiceCategory,
} from "@/lib/mockData";
import { useTranslation } from "react-i18next";
import { Plus, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { Input } from "@/components/ui/input";
import { ServiceLogo } from "./ServiceLogo";
import { categoryKey } from "@/lib/categoryKey";

interface Props {
  onSelect: (service: PopularService) => void;
  onCustom: () => void;
}

type Tab = "All" | ServiceCategory;

export function ServiceCatalog({ onSelect, onCustom }: Props) {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const [tab, setTab] = useState<Tab>("All");

  const tabs: Tab[] = useMemo(() => ["All", ...SERVICE_CATEGORIES], []);

  const filtered = useMemo(() => {
    const query = q.trim().toLowerCase();
    return POPULAR_SERVICES.filter((s) => {
      const matchesTab = tab === "All" || s.category === tab;
      const matchesQuery = !query || s.name.toLowerCase().includes(query);
      return matchesTab && matchesQuery;
    });
  }, [q, tab]);

  return (
    <div className="space-y-4">
      <div className="bg-surface flex items-center gap-3 rounded-2xl px-4 py-3">
        <Search className="h-4 w-4 text-muted-foreground" />
        <Input
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder={t("filter.placeholder")}
          className="h-7 border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0"
        />
      </div>

      <div className="-mx-5 overflow-x-auto px-5 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        <div className="flex gap-2">
          {tabs.map((c) => {
            const active = tab === c;
            return (
              <button
                key={c}
                onClick={() => setTab(c)}
                className={`shrink-0 rounded-full px-3.5 py-1.5 text-xs font-semibold transition-colors ${
                  active
                    ? "bg-gradient-primary text-white shadow-elevated"
                    : "bg-surface text-muted-foreground hover:text-foreground"
                }`}
              >
                {t(categoryKey(c))}
              </button>
            );
          })}
        </div>
      </div>

      <p className="px-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
        {t("modal.popular")}
      </p>

      <div className="relative">
        <div className="max-h-72 overflow-y-auto pr-1">
          {filtered.length > 0 ? (
            <div className="grid grid-cols-3 gap-3 pb-2">
              {filtered.map((s) => (
                <button
                  key={s.id}
                  onClick={() => onSelect(s)}
                  className="bg-surface flex flex-col items-center gap-2 rounded-2xl p-3 transition-transform active:scale-95"
                >
                  <ServiceLogo brand={s.brand} name={s.name} size={48} />
                  <p className="line-clamp-1 text-xs font-semibold">{s.name}</p>
                </button>
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center gap-3 py-10 text-center">
              <p className="text-sm text-muted-foreground">No results</p>
              <button
                onClick={onCustom}
                className="bg-surface rounded-full px-4 py-2 text-xs font-semibold"
              >
                Create custom subscription
              </button>
            </div>
          )}
        </div>
        <div className="from-background pointer-events-none absolute inset-x-0 bottom-0 h-6 bg-gradient-to-t to-transparent" />
      </div>

      <button
        onClick={onCustom}
        className="bg-gradient-primary flex w-full items-center justify-center gap-2 rounded-2xl p-4 text-sm font-semibold text-white shadow-fab transition-transform active:scale-[0.98]"
      >
        <Plus className="h-4 w-4" />
        {t("modal.addCustom")}
      </button>
    </div>
  );
}
