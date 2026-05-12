import { BarChart3, Calendar, Home, Plus, Settings } from "lucide-react";
import { useTranslation } from "react-i18next";

export type TabKey = "dashboard" | "calendar" | "analytics" | "settings";

interface Props {
  active: TabKey;
  onChange: (tab: TabKey) => void;
  onAdd: () => void;
}

export function TabBar({ active, onChange, onAdd }: Props) {
  const { t } = useTranslation();
  const item = (key: TabKey, Icon: typeof Home, label: string) => {
    const isActive = active === key;
    return (
      <button
        key={key}
        onClick={() => onChange(key)}
        className={`flex h-12 w-12 flex-col items-center justify-center rounded-xl transition-colors ${
          isActive ? "bg-primary/15 text-primary" : "text-muted-foreground"
        }`}
        aria-label={label}
      >
        <Icon className="h-5 w-5" />
      </button>
    );
  };

  return (
    <nav className="safe-bottom pointer-events-none fixed inset-x-0 bottom-0 z-40 flex justify-center px-4">
      <div className="bg-surface-elevated/90 pointer-events-auto relative flex w-full max-w-md items-center justify-around rounded-3xl px-3 py-2 shadow-elevated backdrop-blur-xl">
        {item("dashboard", Home, t("nav.dashboard"))}
        {item("calendar", Calendar, t("nav.calendar"))}

        <div className="relative w-14">
          <button
            onClick={onAdd}
            className="bg-gradient-primary absolute -top-7 left-1/2 flex h-14 w-14 -translate-x-1/2 items-center justify-center rounded-full text-white shadow-fab transition-transform active:scale-95"
            aria-label={t("nav.add")}
          >
            <Plus className="h-6 w-6" />
          </button>
        </div>

        {item("analytics", BarChart3, t("nav.analytics"))}
        {item("settings", Settings, t("nav.settings"))}
      </div>
    </nav>
  );
}
