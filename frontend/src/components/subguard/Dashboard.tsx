import { useCallback, useEffect, useMemo, useState } from "react";
import { useDeepLinkHandler } from "@/hooks/use-deep-link";
import { useDebouncedValue } from "@/hooks/use-debounced-value";
import { useNavigate } from "@tanstack/react-router";
import { toast } from "sonner";
import type { Subscription } from "@/types/subscription";
import type { RoomSummary } from "@/types/room";
import type { ServiceCategory } from "@/lib/mockData";
import { convertCurrency } from "@/lib/currencyRates";
import { periodToMonthly } from "@/lib/subscriptionMath";
import { hapticImpact, hapticNotification, hapticSelection, initTelegramApp } from "@/lib/telegram";
import { useTelegramBackButton } from "@/hooks/use-telegram-back";
import { useTranslation } from "react-i18next";
import { SummaryHeader } from "./SummaryHeader";
import { FilterBar } from "./FilterBar";
import { SubscriptionCard } from "./SubscriptionCard";
import { SwipeableSubscriptionCard } from "./SwipeableSubscriptionCard";
import { SponsoredOffers } from "./SponsoredOffers";
import { TabBar, type TabKey } from "./TabBar";

import { AnalyticsView } from "./AnalyticsView";
import { CalendarView } from "./CalendarView";
import { SettingsView } from "./SettingsView";
import { SharedRooms } from "./SharedRooms";
import { OnboardingSheet } from "./OnboardingSheet";
import { EmptyDashboard } from "./EmptyDashboard";
import {
  SummaryHeaderSkeleton,
  DashboardSkeleton,
  CalendarSkeleton,
  AnalyticsSkeleton,
  SettingsSkeleton,
} from "./Skeletons";
import { useShallow } from "zustand/react/shallow";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { useRoomStore } from "@/stores/roomStore";
import { useSettingsStore } from "@/stores/settingsStore";
import { useModalStore } from "@/stores/modalStore";
import { usePaywallStore } from "@/stores/paywallStore";
import { PremiumSheet } from "./PremiumSheet";

interface Props {
  user?: { name: string };
}

export function Dashboard({ user }: Props) {
  // Selector strategy (audit F4): primitive/array state stays granular —
  // Object.is short-circuit catches no-op updates for free. Action +
  // multi-flag clusters that always read together are grouped with
  // useShallow so a single subscription + one shallow compare replaces
  // 5-12 separate Zustand listeners per render.
  const settings = useSettingsStore((s) => s.settings);
  const items = useSubscriptionStore((s) => s.items);
  const filter = useSubscriptionStore((s) => s.filter);
  const sortBy = useSubscriptionStore((s) => s.sortBy);
  const filterTypes = useSubscriptionStore((s) => s.filterTypes);
  const filterCategories = useSubscriptionStore((s) => s.filterCategories);
  const { setFilter, deleteSubscription } = useSubscriptionStore(
    useShallow((s) => ({
      setFilter: s.setFilter,
      deleteSubscription: s.deleteSubscription,
    })),
  );
  const rooms = useRoomStore((s) => s.rooms);
  const fetchRooms = useRoomStore((s) => s.fetchRooms);

  // Modal store: 12 fields, half flags and half action callbacks. They all
  // change together (open/close pairs) so shallow compare is a clean fit.
  const {
    openRoom, closeRoom, activeRoomId,
    openCreateRoom, closeCreateRoom, createRoomOpen,
    openAddSub, closeAddSub, addSubOpen,
    openFilter, closeFilter, filterOpen,
  } = useModalStore(
    useShallow((s) => ({
      openRoom: s.openRoom,
      closeRoom: s.closeRoom,
      activeRoomId: s.activeRoomId,
      openCreateRoom: s.openCreateRoom,
      closeCreateRoom: s.closeCreateRoom,
      createRoomOpen: s.createRoomOpen,
      openAddSub: s.openAddSub,
      closeAddSub: s.closeAddSub,
      addSubOpen: s.addSubOpen,
      openFilter: s.openFilter,
      closeFilter: s.closeFilter,
      filterOpen: s.filterOpen,
    })),
  );

  const [loading, setLoading] = useState(true);
  const [premiumOpen, setPremiumOpen] = useState(false);
  const { t } = useTranslation();
  const navigate = useNavigate();

  const [tab, setTab] = useState<TabKey>("dashboard");

  // Handle Telegram deep link — auto-open room after join
  useDeepLinkHandler(useCallback((roomId: string) => {
    openRoom(roomId);
  }, [openRoom]));

  // Init Telegram + fetch rooms/paywall config on mount
  const fetchConfig = usePaywallStore((s) => s.fetchConfig);
  useEffect(() => {
    initTelegramApp();
    fetchRooms();
    fetchConfig();
    const timer = setTimeout(() => setLoading(false), 800);
    return () => clearTimeout(timer);
  }, [fetchRooms, fetchConfig]);

  // Room detail fetching is handled by GlobalModals

  // Show Telegram BackButton when not on dashboard tab, or when a sheet is open
  const isSubPage = tab !== "dashboard" || addSubOpen || !!activeRoomId || createRoomOpen;
  const handleBack = useCallback(() => {
    if (addSubOpen) {
      closeAddSub();
      hapticImpact("light");
    } else if (createRoomOpen) {
      closeCreateRoom();
      hapticImpact("light");
    } else if (activeRoomId) {
      closeRoom();
      hapticImpact("light");
    } else if (tab !== "dashboard") {
      setTab("dashboard");
      hapticImpact("light");
    }
  }, [addSubOpen, closeAddSub, createRoomOpen, closeCreateRoom, activeRoomId, closeRoom, tab]);

  useTelegramBackButton(isSubPage, handleBack);

  // Debounce search so typing isn't laggy on long subscription lists.
  const debouncedFilter = useDebouncedValue(filter, 200);

  // ── Filter pipeline ────────────────────────────────────
  // Order matters: search → type → category → sort. Each step shrinks the
  // working set so the final sort runs on the smallest possible array.
  //
  // The `type` filter is interpreted as "user-asked-for-these-kinds": if
  // they DID check boxes, only those kinds render. If the box set is empty
  // (default state) it's a no-op and everything stays.
  const showSubscriptions =
    filterTypes.length === 0 || filterTypes.includes("subscription");
  const showRooms =
    filterTypes.length === 0 || filterTypes.includes("room");

  const filteredSubscriptions = useMemo(() => {
    if (!showSubscriptions) return [];

    const q = debouncedFilter.trim().toLowerCase();
    const categorySet = new Set<ServiceCategory>(filterCategories);

    let list = items.slice();

    // 1) text search across name, tag, note
    if (q) {
      list = list.filter(
        (s) =>
          s.name.toLowerCase().includes(q) ||
          (s.tag ?? "").toLowerCase().includes(q) ||
          (s.note ?? "").toLowerCase().includes(q),
      );
    }

    // 2) category — we reuse the `tag` field on Subscription which is
    //    populated with ServiceCategory when picked from the catalog.
    if (categorySet.size > 0) {
      list = list.filter((s) => s.tag && categorySet.has(s.tag as ServiceCategory));
    }

    // 3) sort
    switch (sortBy) {
      case "priceDesc":
        list.sort((a, b) => b.amount - a.amount);
        break;
      case "priceAsc":
        list.sort((a, b) => a.amount - b.amount);
        break;
      case "alphabetical":
        list.sort((a, b) => a.name.localeCompare(b.name));
        break;
      case "nextPayment":
      default:
        list.sort(
          (a, b) =>
            new Date(a.next_payment_at).getTime() -
            new Date(b.next_payment_at).getTime(),
        );
        break;
    }
    return list;
  }, [items, debouncedFilter, filterCategories, sortBy, showSubscriptions]);

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

  // ── Paywall gate ─────────────────────────────────────
  const paywallConfig = usePaywallStore((s) => s.config);

  const isPaywalled = (resource: "subs" | "rooms"): boolean => {
    if (!paywallConfig.paywall_enabled) return false;
    if (settings.isSubscribed) return false; // premium user
    if (resource === "subs") return items.length >= paywallConfig.free_subs_limit;
    return rooms.length >= paywallConfig.free_room_limit;
  };

  const openAdd = () => {
    if (isPaywalled("subs")) {
      setPremiumOpen(true);
      hapticNotification("warning");
      return;
    }
    openAddSub(null);
    hapticImpact("medium");
  };
  const openEdit = (s: Subscription) => {
    openAddSub(s as unknown as Record<string, unknown>);
    hapticImpact("light");
  };

  const handleDelete = async (id: string) => {
    try {
      await deleteSubscription(id);
      hapticNotification("warning");
    } catch {
      hapticNotification("error");
    }
  };

  const handleTabChange = (newTab: TabKey) => {
    setTab(newTab);
    hapticSelection();
  };

  const handleViewAllRooms = useCallback(() => navigate({ to: "/rooms" }), [navigate]);
  const handleOpenRoom = useCallback((r: RoomSummary) => {
    openRoom(r.id);
    hapticImpact("light");
  }, [openRoom]);
  const handleCreateRoom = useCallback(() => {
    if (!paywallConfig.paywall_enabled || settings.isSubscribed || rooms.length < paywallConfig.free_room_limit) {
      openCreateRoom();
      hapticImpact("medium");
    } else {
      setPremiumOpen(true);
      hapticNotification("warning");
    }
  }, [openCreateRoom, paywallConfig, settings.isSubscribed, rooms.length]);

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
              {showRooms && (
                <SharedRooms
                  rooms={rooms}
                  onViewAll={handleViewAllRooms}
                  onOpen={handleOpenRoom}
                  onCreateRoom={handleCreateRoom}
                />
              )}
              {items.length === 0 ? (
                <EmptyDashboard onAdd={openAdd} />
              ) : (
                <>
                  <div className="mt-2">
                    <FilterBar
                      value={filter}
                      onChange={setFilter}
                      onOpenFilters={() => openFilter()}
                    />
                  </div>
                  {showSubscriptions && (
                    <div className="mt-5 space-y-2 px-5">
                      {filteredSubscriptions.map((s) => (
                        <SwipeableSubscriptionCard
                          key={s.id}
                          subscription={s}
                          onClick={openEdit}
                          onDelete={handleDelete}
                        />
                      ))}
                      {filteredSubscriptions.length === 0 && (
                        <p className="py-10 text-center text-sm text-muted-foreground">
                          {t("dashboard.noResults")}
                        </p>
                      )}
                    </div>
                  )}
                  <div className="mt-8">
                    <SponsoredOffers />
                  </div>
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
        <PremiumSheet open={premiumOpen} onClose={() => setPremiumOpen(false)} />
      </div>
    </>
  );
}
