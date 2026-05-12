import { createFileRoute } from "@tanstack/react-router";
import { AdminPanel } from "@/components/subguard/AdminPanel";

export const Route = createFileRoute("/admin")({
  component: AdminPanel,
});
