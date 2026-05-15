import { createFileRoute } from "@tanstack/react-router";
import { useEffect } from "react";
import { Dashboard } from "@/components/subguard/Dashboard";
import { useSettingsStore } from "@/stores/settingsStore";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { useRoomStore } from "@/stores/roomStore";
import { usePaywallStore } from "@/stores/paywallStore";

export const Route = createFileRoute("/")({
  component: Index,
});

function Index() {
  const { fetchProfile, user } = useSettingsStore();
  const { fetchSubscriptions } = useSubscriptionStore();
  const { fetchRooms } = useRoomStore();
  const { fetchConfig } = usePaywallStore();

  // Load all data on mount. Zustand action refs are stable but listing
  // them in deps trips HMR / strict-mode into double-fetching; we want
  // a single boot fetch, hence the empty deps array.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    fetchProfile();
    fetchSubscriptions();
    fetchRooms();
    fetchConfig();
  }, []);

  return (
    <Dashboard
      user={user ? { name: user.name } : undefined}
    />
  );
}
