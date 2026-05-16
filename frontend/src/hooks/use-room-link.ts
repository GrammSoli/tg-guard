import { useEffect, useRef } from "react";

/**
 * Reads the `room` query param from the URL once per session and opens that
 * room. Set by the "Перейти в комнату" button on bot payment-reminder
 * messages — a Telegram web_app button whose URL is `<app>/?room=<roomId>`.
 */
export function useRoomLinkHandler(onRoom: (roomId: string) => void) {
  const processed = useRef(false);

  useEffect(() => {
    if (processed.current) return;
    processed.current = true;

    const roomId = new URLSearchParams(window.location.search).get("room");
    if (roomId) onRoom(roomId);
  }, [onRoom]);
}
