import { useEffect } from "react";
import { showBackButton, hideBackButton } from "@/lib/telegram";

/**
 * Show the Telegram BackButton while this component is mounted.
 * Calls `onBack` when pressed. Hides on unmount.
 */
export function useTelegramBackButton(visible: boolean, onBack: () => void) {
  useEffect(() => {
    if (!visible) {
      hideBackButton();
      return;
    }
    showBackButton(onBack);
    return () => hideBackButton();
  }, [visible, onBack]);
}
