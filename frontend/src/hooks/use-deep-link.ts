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
  const { join, fetchRooms } = useRoomStore();
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
