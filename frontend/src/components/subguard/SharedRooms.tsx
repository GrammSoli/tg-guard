import { ArrowRight, Plus, Users } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { RoomSummary } from "@/types/room";
import { formatCurrency, localeFor } from "@/lib/format";
import { convertCurrency } from "@/lib/currencyRates";
import { useSettingsStore } from "@/stores/settingsStore";
import { useDragScroll } from "@/hooks/useDragScroll";
import { ServiceLogo } from "./ServiceLogo";

interface Props {
  rooms: RoomSummary[];
  onViewAll?: () => void;
  onOpen?: (room: RoomSummary) => void;
  onCreateRoom?: () => void;
}

export function SharedRooms({ rooms, onViewAll, onOpen, onCreateRoom }: Props) {
  const { t, i18n } = useTranslation();
  const lc = localeFor(i18n.language);
  const { settings } = useSettingsStore();
  const userCurrency = settings.defaultCurrency;
  const dragScrollRef = useDragScroll<HTMLDivElement>();

  return (
    <section className="mt-6">
      <div className="mb-3 flex items-center justify-between px-5">
        <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
          {t("dashboard.sharedRooms")}
        </p>
        <button
          onClick={onViewAll}
          className="flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          {t("dashboard.viewAll")} <ArrowRight className="h-3 w-3" />
        </button>
      </div>

      <div
        ref={dragScrollRef}
        className="no-scrollbar flex space-x-3 overflow-x-auto px-5 pb-3 select-none cursor-grab active:cursor-grabbing"
      >
        {/* Create room card */}
        <button
          onClick={onCreateRoom}
          className="bg-surface hover:bg-surface-elevated flex h-24 w-48 shrink-0 flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-white/20 transition-colors"
        >
          <div className="bg-primary/15 flex h-8 w-8 items-center justify-center rounded-full">
            <Plus className="h-4 w-4 text-primary" />
          </div>
          <p className="text-xs font-semibold text-muted-foreground">{t("dashboard.newRoom")}</p>
        </button>

        {rooms.map((room) => {
          const rawAmount = typeof room.total_per_member === "number" && isFinite(room.total_per_member)
            ? room.total_per_member
            : 0;
          // Convert from room currency to user's default currency
          const displayAmount = convertCurrency(rawAmount, room.currency, userCurrency);

          return (
            <button
              key={room.id}
              onClick={() => onOpen?.(room)}
              className="bg-surface hover:bg-surface-elevated flex h-24 w-60 shrink-0 flex-col justify-between rounded-xl border border-white/10 p-4 text-left transition-colors"
            >
              <div className="flex items-center justify-between">
                <p className="truncate text-sm font-bold">{room.name}</p>
                <div className="flex -space-x-1.5">
                  {room.services.slice(0, 3).map((s, i) => (
                    <ServiceLogo
                      key={i}
                      brand={s.brand}
                      name={s.brand}
                      size={20}
                      rounded="full"
                      className="border border-background"
                    />
                  ))}
                </div>
              </div>
              <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
                <Users className="h-3 w-3" />
                <span>{t("dashboard.members", { count: room.members })}</span>
                <span className="opacity-50">•</span>
                <span className="font-medium text-foreground">
                  {formatCurrency(displayAmount, userCurrency, lc)} {t("dashboard.perMonth")}
                </span>
              </div>
            </button>
          );
        })}
      </div>
    </section>
  );
}
