import { createFileRoute } from "@tanstack/react-router";
import { useEffect } from "react";
import { Dashboard } from "@/components/subguard/Dashboard";
import { useSettingsStore } from "@/stores/settingsStore";
import { useSubscriptionStore } from "@/stores/subscriptionStore";
import { useRoomStore } from "@/stores/roomStore";
import { usePaywallStore } from "@/stores/paywallStore";
import { useFxStore } from "@/stores/fxStore";

export const Route = createFileRoute("/")({
  component: Index,
});

function Index() {
  // Granular selectors — every destructured `useStore()` here used to
  // subscribe Index to the WHOLE store. A mark-paid on a different
  // route, a settings PATCH, a sub list refresh — all of them would
  // re-render Index, which would then re-render Dashboard with the
  // same `user` prop. Each Zustand action is a stable ref, so
  // selecting them individually triggers zero re-renders. `user`
  // genuinely changes (after fetchProfile lands the profile), so it
  // remains a granular selector — only that change re-renders Index.
  // Audit #18.
  const fetchProfile = useSettingsStore((s) => s.fetchProfile);
  const user = useSettingsStore((s) => s.user);
  const fetchSubscriptions = useSubscriptionStore((s) => s.fetchSubscriptions);
  const fetchRooms = useRoomStore((s) => s.fetchRooms);
  const fetchConfig = usePaywallStore((s) => s.fetchConfig);
  const fetchRates = useFxStore((s) => s.fetchRates);

  // Load all data on mount. Zustand action refs are stable but listing
  // them in deps trips HMR / strict-mode into double-fetching; we want
  // a single boot fetch, hence the empty deps array.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    fetchProfile();
    fetchSubscriptions();
    fetchRooms();
    fetchConfig();
    fetchRates();
  }, []);

  return (
    <Dashboard
      user={user ? { name: user.name } : undefined}
    />
  );
}
