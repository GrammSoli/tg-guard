import { useCallback, useEffect, useMemo, useState } from "react";
import { useDeepLinkHandler } from "@/hooks/use-deep-link";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import type { PartnerOffer, Subscription } from "@/types/subscription";
import type { RoomSummary } from "@/types/room";
import { convertCurrency } from "@/lib/currencyRates";
import { periodToMonthly } from "@/lib/subscriptionMath";
import { hapticImpact, hapticNotification, hapticSelection, initTelegramApp } from "@/lib/telegram";
import { useTelegramBackButton } from "@/hooks/use-telegram-back";
import { useTranslation } from "react-i18next";
import { SummaryHeader } from "./SummaryHeader";
import { FilterBar } from "./FilterBar";
import { SubscriptionCard } from "./SubscriptionCard";
import { SwipeableSubscriptionCard } from "./SwipeableSubscriptionCard";
import { PartnerOffers } from "./PartnerOffers";
import { TabBar, type TabKey } from "./TabBar";
import { AddSubscriptionSheet } from "./AddSubscriptionSheet";
import { AnalyticsView } from "./AnalyticsView";
import { CalendarView } from "./CalendarView";
import { SettingsView } from "./SettingsView";
import { SharedRooms } from "./SharedRooms";
import { SharedRoomSheet } from "./SharedRoomSheet";
import { CreateRoomSheet } from "./CreateRoomSheet";
import { OnboardingSheet } from "./OnboardingSheet";
import { EmptyDashboard } from "./EmptyDashboard";
import {
  SummaryHeaderSkeleton,
  DashboardSkeleton,
  CalendarSkeleton,
  AnalyticsSkeleton,
  SettingsSkeleton,
} from "./Skeletons";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { useRoomStore } from "@/stores/roomStore";
import { useSettingsStore } from "@/stores/settingsStore";

interface Props {
  partnerOffers: PartnerOffer[];
  user?: { name: string };
}

export function Dashboard({ partnerOffers, user }: Props) {
  // Granular Zustand selectors so unrelated store updates (e.g. activeDetail
  // changing) don't re-render the dashboard.
  const settings = useSettingsStore((s) => s.settings);
  const items = useSubscriptionStore((s) => s.items);
  const filter = useSubscriptionStore((s) => s.filter);
  const setFilter = useSubscriptionStore((s) => s.setFilter);
  const addSubscription = useSubscriptionStore((s) => s.addSubscription);
  const updateSubscription = useSubscriptionStore((s) => s.updateSubscription);
  const deleteSubscription = useSubscriptionStore((s) => s.deleteSubscription);
  const rooms = useRoomStore((s) => s.rooms);
  const fetchRooms = useRoomStore((s) => s.fetchRooms);
  const fetchDetail = useRoomStore((s) => s.fetchDetail);

  const [loading, setLoading] = useState(true);
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [tab, setTab] = useState<TabKey>("dashboard");
  const [sheetOpen, setSheetOpen] = useState(false);
  const [editing, setEditing] = useState<Subscription | null>(null);
  const [activeRoomId, setActiveRoomId] = useState<string | null>(null);
  const [createRoomOpen, setCreateRoomOpen] = useState(false);

  // Handle Telegram deep link — auto-open room after join
  useDeepLinkHandler(useCallback((roomId: string) => {
    setActiveRoomId(roomId);
  }, []));

  // Init Telegram + fetch rooms on mount
  useEffect(() => {
    initTelegramApp();
    fetchRooms();
    const timer = setTimeout(() => setLoading(false), 800);
    return () => clearTimeout(timer);
  }, [fetchRooms]);

  // When a room is selected, fetch its details
  useEffect(() => {
    if (activeRoomId) fetchDetail(activeRoomId);
  }, [activeRoomId, fetchDetail]);

  // Show Telegram BackButton when not on dashboard tab, or when a sheet is open
  const isSubPage = tab !== "dashboard" || sheetOpen || !!activeRoomId || createRoomOpen;
  const handleBack = useCallback(() => {
    if (sheetOpen) {
      setSheetOpen(false);
      setEditing(null);
      hapticImpact("light");
    } else if (createRoomOpen) {
      setCreateRoomOpen(false);
      hapticImpact("light");
    } else if (activeRoomId) {
      setActiveRoomId(null);
      hapticImpact("light");
    } else if (tab !== "dashboard") {
      setTab("dashboard");
      hapticImpact("light");
    }
  }, [sheetOpen, createRoomOpen, activeRoomId, tab]);

  useTelegramBackButton(isSubPage, handleBack);

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (s) =>
        s.name.toLowerCase().includes(q) ||
        (s.tag ?? "").toLowerCase().includes(q),
    );
  }, [items, filter]);

  const totalMonthly = useMemo(
    () =>
      items
        .filter((s) => !s.is_trial)
        .reduce(
          (sum, s) =>
            sum + convertCurrency(periodToMonthly(s), s.currency, settings.defaultCurrency),
          0,
        ),
    [items, settings.defaultCurrency],
  );

  const handleSave = (data: Omit<Subscription, "id"> & { id?: string }) => {
    if (data.id) {
      updateSubscription(data.id, data);
    } else {
      addSubscription(data as Omit<Subscription, "id">);
    }
    setEditing(null);
    hapticNotification("success");
  };

  const handleDelete = (id: string) => {
    deleteSubscription(id);
    setSheetOpen(false);
    setEditing(null);
    hapticNotification("warning");
  };

  const openAdd = () => {
    setEditing(null);
    setSheetOpen(true);
    hapticImpact("medium");
  };
  const openEdit = (s: Subscription) => {
    setEditing(s);
    setSheetOpen(true);
    hapticImpact("light");
  };

  const handleTabChange = (newTab: TabKey) => {
    setTab(newTab);
    hapticSelection();
  };

  const handleViewAllRooms = useCallback(() => navigate({ to: "/rooms" }), [navigate]);
  const handleOpenRoom = useCallback((r: RoomSummary) => {
    setActiveRoomId(r.id);
    hapticImpact("light");
  }, []);
  const handleCreateRoom = useCallback(() => {
    setCreateRoomOpen(true);
    hapticImpact("medium");
  }, []);

  return (
    <>
      <div className="bg-background min-h-[100dvh] pb-32">
        {tab !== "settings" && (
          loading ? (
            <SummaryHeaderSkeleton />
          ) : (
            <SummaryHeader
              activeCount={items.length}
              totalMonthly={totalMonthly}
              currency={settings.defaultCurrency}
              user={user}
              settings={settings}
            />
          )
        )}

        {tab === "dashboard" && (
          loading ? (
            <DashboardSkeleton />
          ) : (
            <>
              <SharedRooms
                rooms={rooms}
                onViewAll={handleViewAllRooms}
                onOpen={handleOpenRoom}
                onCreateRoom={handleCreateRoom}
              />
              {items.length === 0 ? (
                <EmptyDashboard onAdd={openAdd} />
              ) : (
                <>
                  <div className="mt-2">
                    <FilterBar value={filter} onChange={setFilter} />
                  </div>
                  <div className="mt-5 space-y-2 px-5">
                    {filtered.map((s) => (
                      <SwipeableSubscriptionCard
                        key={s.id}
                        subscription={s}
                        onClick={openEdit}
                        onDelete={handleDelete}
                      />
                    ))}
                    {filtered.length === 0 && (
                      <p className="py-10 text-center text-sm text-muted-foreground">
                        {t("dashboard.noResults")}
                      </p>
                    )}
                  </div>
                  {settings.cpaActive && (
                    <div className="mt-8">
                      <PartnerOffers offers={partnerOffers} />
                    </div>
                  )}
                </>
              )}
            </>
          )
        )}

        {tab === "calendar" && (
          <div className="mt-2">
            {loading ? <CalendarSkeleton /> : <CalendarView subscriptions={items} onEdit={openEdit} />}
          </div>
        )}

        {tab === "analytics" && (
          <div className="mt-2">
            {loading ? <AnalyticsSkeleton /> : <AnalyticsView subscriptions={items} currency={settings.defaultCurrency} />}
          </div>
        )}

        {tab === "settings" && (
          <div className="mt-2">
            {loading ? <SettingsSkeleton /> : <SettingsView settings={settings} user={user} />}
          </div>
        )}

        <TabBar
          active={tab}
          onChange={handleTabChange}
          onAdd={openAdd}
        />

        <OnboardingSheet />

        <AddSubscriptionSheet
          open={sheetOpen}
          onOpenChange={setSheetOpen}
          initial={editing}
          onSave={handleSave}
          onDelete={undefined}
        />

        <SharedRoomSheet
          roomId={activeRoomId}
          open={!!activeRoomId}
          onOpenChange={(o) => {
            if (!o) {
              setActiveRoomId(null);
              hapticImpact("light");
            }
          }}
        />

        <CreateRoomSheet
          open={createRoomOpen}
          onOpenChange={setCreateRoomOpen}
        />
      </div>
    </>
  );
}
