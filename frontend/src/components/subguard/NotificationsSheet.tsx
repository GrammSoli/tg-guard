import { useTranslation } from "react-i18next";
import { Bell } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { useSettingsStore } from "@/stores/settingsStore";
import { hapticSelection } from "@/lib/telegram";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * NotificationsSheet — bottom sheet with a single toggle for "send me payment
 * reminders". Wired through `useSettingsStore.updateSettings`, which already
 * does optimistic-update + rollback + toast.error on PATCH /me failure, so
 * the Switch just calls into it.
 */
export function NotificationsSheet({ open, onOpenChange }: Props) {
  const { t } = useTranslation();
  const notificationsEnabled = useSettingsStore(
    (s) => s.settings.notificationsEnabled,
  );
  const updateSettings = useSettingsStore((s) => s.updateSettings);

  const handleToggle = (checked: boolean) => {
    hapticSelection();
    updateSettings({ notificationsEnabled: checked });
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="rounded-t-3xl">
        <SheetHeader className="text-left">
          <SheetTitle>{t("notifications.sheetTitle")}</SheetTitle>
          <SheetDescription>{t("notifications.sheetDesc")}</SheetDescription>
        </SheetHeader>

        <div className="mt-4 pb-6">
          <label
            htmlFor="notifications-toggle"
            className="bg-surface flex w-full cursor-pointer items-center gap-3 rounded-2xl p-4 text-left"
          >
            <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
              <Bell className="h-4 w-4" />
            </div>
            <div className="flex-1">
              <p className="text-sm font-semibold">
                {t("notifications.toggleLabel")}
              </p>
              <p className="text-xs text-muted-foreground">
                {t("notifications.toggleHint")}
              </p>
            </div>
            <Switch
              id="notifications-toggle"
              checked={notificationsEnabled}
              onCheckedChange={handleToggle}
            />
          </label>
        </div>
      </SheetContent>
    </Sheet>
  );
}
