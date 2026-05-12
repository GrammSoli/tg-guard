import { useTranslation } from "react-i18next";
import { Search, SlidersHorizontal, X } from "lucide-react";

interface Props {
  value: string;
  onChange: (v: string) => void;
}

export function FilterBar({ value, onChange }: Props) {
  const { t } = useTranslation();
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
          >
            <X className="h-3 w-3" />
          </button>
        )}
        <SlidersHorizontal className="h-4 w-4 text-muted-foreground" />
      </div>
    </div>
  );
}
