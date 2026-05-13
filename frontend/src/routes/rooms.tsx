import { useEffect, useMemo, useState } from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, ArrowUpDown, Plus, Search, Users, X } from "lucide-react";
import { useRoomStore } from "@/stores/roomStore";
import { useSettingsStore } from "@/stores/settingsStore";
import { useModalStore } from "@/stores/modalStore";
import { formatCurrency, localeFor } from "@/lib/format";
import { convertCurrency } from "@/lib/currencyRates";
import { SwipeableRoomCard } from "@/components/subguard/SwipeableRoomCard";
import { hapticImpact } from "@/lib/telegram";

export const Route = createFileRoute("/rooms")({
  component: RoomsPage,
  head: () => ({
    meta: [
      { title: "All Rooms — SubGuard" },
      { name: "description", content: "Browse and manage all your shared subscription rooms." },
      { property: "og:title", content: "All Rooms — SubGuard" },
      { property: "og:description", content: "Browse and manage all your shared subscription rooms." },
    ],
  }),
});

type SortKey = "name" | "members" | "cost";
const SORT_STORAGE_KEY = "rooms.sort";

function RoomsPage() {
  const { t, i18n } = useTranslation();
  const lc = localeFor(i18n.language);
  const { rooms, fetchRooms, deleteRoom } = useRoomStore();
  const { settings } = useSettingsStore();
  const openRoom = useModalStore((s) => s.openRoom);
  const openCreateRoom = useModalStore((s) => s.openCreateRoom);
  const userCurrency = settings.defaultCurrency;
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortKey>(() => {
    if (typeof window === "undefined") return "name";
    const stored = window.localStorage.getItem(SORT_STORAGE_KEY);
    return stored === "members" || stored === "cost" || stored === "name" ? stored : "name";
  });

  useEffect(() => {
    fetchRooms();
  }, [fetchRooms]);

  useEffect(() => {
    if (typeof window !== "undefined") {
      window.localStorage.setItem(SORT_STORAGE_KEY, sort);
    }
  }, [sort]);

  const filteredRooms = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? rooms.filter(
          (r) =>
            r.name.toLowerCase().includes(q) ||
            r.services.some((s) => String(s.brand).toLowerCase().includes(q)),
        )
      : rooms;
    const sorted = [...list];
    if (sort === "name") {
      sorted.sort((a, b) => a.name.localeCompare(b.name));
    } else if (sort === "members") {
      sorted.sort((a, b) => b.members - a.members);
    } else {
      sorted.sort((a, b) => (b.total_per_member ?? 0) - (a.total_per_member ?? 0));
    }
    return sorted;
  }, [rooms, query, sort]);

  return (
    <div className="bg-background min-h-screen pb-32">
      <header className="flex items-center justify-between px-5 pt-6 pb-4">
        <button
          onClick={() => {
            hapticImpact("light");
            navigate({ to: "/" });
          }}
          className="bg-surface hover:bg-surface-elevated flex h-9 w-9 items-center justify-center rounded-full transition-colors"
          aria-label="Back"
        >
          <ArrowLeft className="h-4 w-4" />
        </button>
        <h1 className="text-base font-bold">{t("rooms.allRooms")}</h1>
        <button
          onClick={() => {
            hapticImpact("medium");
            openCreateRoom();
          }}
          className="bg-primary/15 hover:bg-primary/25 flex h-9 w-9 items-center justify-center rounded-full text-primary transition-colors"
          aria-label="Create room"
        >
          <Plus className="h-4 w-4" />
        </button>
      </header>

      {rooms.length > 0 && (
        <div className="px-5 pb-4">
          <div className="bg-surface-elevated flex items-center gap-3 rounded-2xl px-4 py-3">
            <Search className="h-4 w-4 text-muted-foreground" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("rooms.searchPlaceholder")}
              className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
            />
            {query && (
              <button
                onClick={() => setQuery("")}
                className="flex h-5 w-5 items-center justify-center rounded-full bg-muted-foreground/20 text-muted-foreground transition-colors hover:bg-muted-foreground/30"
                aria-label="Clear search"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
          <div className="mt-3 flex items-center gap-2">
            <ArrowUpDown className="h-3.5 w-3.5 text-muted-foreground" />
            <span className="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">
              {t("rooms.sort")}
            </span>
            <div className="bg-surface-elevated ml-auto flex items-center gap-1 rounded-full p-1">
              {([
                { key: "name", labelKey: "rooms.sortName" },
                { key: "members", labelKey: "rooms.sortMembers" },
                { key: "cost", labelKey: "rooms.sortCost" },
              ] as { key: SortKey; labelKey: string }[]).map((opt) => (
                <button
                  key={opt.key}
                  onClick={() => {
                    hapticImpact("light");
                    setSort(opt.key);
                  }}
                  className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors ${
                    sort === opt.key
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  {t(opt.labelKey)}
                </button>
              ))}
            </div>
          </div>
        </div>
      )}

      <section className="space-y-2 px-5">
        {rooms.length === 0 ? (
          <div className="bg-surface flex flex-col items-center gap-3 rounded-xl border border-white/10 p-8 text-center">
            <div className="bg-primary/15 flex h-12 w-12 items-center justify-center rounded-full">
              <Users className="h-5 w-5 text-primary" />
            </div>
            <p className="text-sm font-semibold">{t("rooms.noRoomsYet")}</p>
            <p className="text-xs text-muted-foreground">
              {t("rooms.noRoomsHint")}
            </p>
            <button
              onClick={() => openCreateRoom()}
              className="bg-primary text-primary-foreground hover:bg-primary/90 mt-2 rounded-full px-4 py-2 text-xs font-semibold transition-colors"
            >
              {t("createRoom.create")}
            </button>
          </div>
        ) : filteredRooms.length === 0 ? (
          <div className="bg-surface flex flex-col items-center gap-2 rounded-xl border border-white/10 p-6 text-center">
            <p className="text-sm font-semibold">{t("rooms.nothingFound")}</p>
            <p className="text-xs text-muted-foreground">
              {t("rooms.nothingFoundHint")}
            </p>
          </div>
        ) : (
          filteredRooms.map((room) => {
            const rawAmount = typeof room.total_per_member === "number" && isFinite(room.total_per_member)
              ? room.total_per_member
              : 0;
            const displayAmount = convertCurrency(rawAmount, room.currency, userCurrency);

            return (
              <SwipeableRoomCard
                key={room.id}
                room={room}
                displayAmount={displayAmount}
                userCurrency={userCurrency}
                onClick={(id) => openRoom(id)}
                onDelete={(id) => deleteRoom(id)}
              />
            );
          })
        )}
      </section>


      <Link to="/" className="sr-only">
        Home
      </Link>
    </div>
  );
}
