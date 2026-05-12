import { Inbox, Plus } from "lucide-react";
import { useTranslation } from "react-i18next";

interface Props {
  onAdd: () => void;
}

export function EmptyDashboard({ onAdd }: Props) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col items-center px-8 py-12 text-center">
      <div className="bg-surface mb-5 flex h-20 w-20 items-center justify-center rounded-3xl">
        <Inbox className="h-9 w-9 text-muted-foreground" />
      </div>
      <p className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
        {t("dashboard.noSubscriptions")}
      </p>
      <h3 className="mt-2 max-w-xs text-lg font-bold">
        {t("dashboard.noSubscriptionsHint")}
      </h3>
      <button
        onClick={onAdd}
        className="bg-gradient-primary shadow-elevated mt-6 inline-flex items-center gap-2 rounded-2xl px-5 py-3 text-sm font-semibold text-white transition-transform active:scale-[0.98]"
      >
        <Plus className="h-4 w-4" />
        {t("dashboard.addFirst")}
      </button>
    </div>
  );
}
