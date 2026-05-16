import { createFileRoute } from "@tanstack/react-router";
import { lazy, Suspense } from "react";

// The /rooms page is a secondary route — lazy-load it so its component
// tree (and SwipeableRoomCard) stays out of the entry chunk.
const RoomsPage = lazy(() => import("@/components/subguard/RoomsPageView"));

function RoomsRoute() {
  return (
    <Suspense fallback={<div className="bg-background min-h-screen" />}>
      <RoomsPage />
    </Suspense>
  );
}

export const Route = createFileRoute("/rooms")({
  component: RoomsRoute,
  head: () => ({
    meta: [
      { title: "All Rooms — SubGuard" },
      { name: "description", content: "Browse and manage all your shared subscription rooms." },
      { property: "og:title", content: "All Rooms — SubGuard" },
      {
        property: "og:description",
        content: "Browse and manage all your shared subscription rooms.",
      },
    ],
  }),
});
