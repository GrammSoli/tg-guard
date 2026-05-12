import { createFileRoute } from "@tanstack/react-router";
import { useEffect } from "react";
import { Dashboard } from "@/components/subguard/Dashboard";
import { useSettingsStore } from "@/stores/settingsStore";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { useRoomStore } from "@/stores/roomStore";

export const Route = createFileRoute("/")({
  component: Index,
});

function Index() {
  const { fetchProfile, user } = useSettingsStore();
  const { fetchSubscriptions } = useSubscriptionStore();
  const { fetchRooms } = useRoomStore();

  // Load all data on mount
  useEffect(() => {
    fetchProfile();
    fetchSubscriptions();
    fetchRooms();
  }, [fetchProfile, fetchSubscriptions, fetchRooms]);

  return (
    <Dashboard
      partnerOffers={[]}
      user={user ? { name: user.name } : undefined}
    />
  );
}
