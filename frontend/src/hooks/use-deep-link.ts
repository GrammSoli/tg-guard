import { useEffect, useRef } from "react";
import { useRoomStore } from "@/stores/roomStore";
import { hapticNotification } from "@/lib/telegram";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

/**
 * Reads `start_param` from Telegram WebApp initData once per session.
 * If it matches `room_<inviteCode>`, automatically joins the room.
 */
export function useDeepLinkHandler(onJoined?: (roomId: string) => void) {
  const processed = useRef(false);
  // Granular selectors — was destructuring useRoomStore() which
  // subscribed the host component (Dashboard) to EVERY roomStore
  // mutation: mark-paid, fetchDetail, fetchRooms refresh, etc. The
  // actions themselves are stable refs in Zustand, so selecting them
  // individually means the host re-renders ZERO times due to this hook.
  // Audit #18.
  const join = useRoomStore((s) => s.join);
  const fetchRooms = useRoomStore((s) => s.fetchRooms);
  const { t } = useTranslation();

  useEffect(() => {
    if (processed.current) return;
    processed.current = true;

    const startParam =
      (window as any).Telegram?.WebApp?.initDataUnsafe?.start_param as
        | string
        | undefined;

    if (!startParam || !startParam.startsWith("room_")) return;

    const inviteCode = startParam.replace("room_", "");
    if (!inviteCode) return;

    const toastId = toast.loading(t("deeplink.joining"));

    (async () => {
      try {
        const room = await join(inviteCode);
        await fetchRooms();
        hapticNotification("success");
        toast.success(t("deeplink.joined", { name: room.name }), { id: toastId });
        onJoined?.(room.id);
      } catch {
        hapticNotification("error");
        toast.error(t("deeplink.failed"), { id: toastId });
      }
    })();
  }, [join, fetchRooms, t, onJoined]);
}
