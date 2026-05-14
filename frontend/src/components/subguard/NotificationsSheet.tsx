import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Bell, Clock, Globe } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useSettingsStore } from "@/stores/settingsStore";
import { hapticSelection } from "@/lib/telegram";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

// Curated list of reasonable notification times. Less choice paralysis than
// a full <input type="time">; the backend accepts any HH:MM if we ever
// expand this. Times are in the user's local timezone.
const TIME_OPTIONS = [
  "08:00",
  "09:00",
  "10:00",
  "12:00",
  "15:00",
  "18:00",
  "20:00",
  "21:00",
];

const TIMEZONE_OPTIONS: { value: string; ru: string; en: string }[] = [
  { value: "Pacific/Honolulu", ru: "Гонолулу (UTC-10)", en: "Honolulu (UTC-10)" },
  { value: "America/Anchorage", ru: "Аляска (UTC-9)", en: "Alaska (UTC-9)" },
  { value: "America/Los_Angeles", ru: "Лос-Анджелес (UTC-8)", en: "Los Angeles (UTC-8)" },
  { value: "America/Denver", ru: "Денвер (UTC-7)", en: "Denver (UTC-7)" },
  { value: "America/Chicago", ru: "Чикаго (UTC-6)", en: "Chicago (UTC-6)" },
  { value: "America/New_York", ru: "Нью-Йорк (UTC-5)", en: "New York (UTC-5)" },
  { value: "America/Sao_Paulo", ru: "Сан-Паулу (UTC-3)", en: "São Paulo (UTC-3)" },
  { value: "Atlantic/Reykjavik", ru: "Рейкьявик (UTC+0)", en: "Reykjavik (UTC+0)" },
  { value: "Europe/London", ru: "Лондон (UTC+0/+1)", en: "London (UTC+0/+1)" },
  { value: "Europe/Berlin", ru: "Берлин (UTC+1/+2)", en: "Berlin (UTC+1/+2)" },
  { value: "Europe/Istanbul", ru: "Стамбул (UTC+3)", en: "Istanbul (UTC+3)" },
  { value: "Europe/Moscow", ru: "Москва (UTC+3)", en: "Moscow (UTC+3)" },
  { value: "Europe/Samara", ru: "Самара (UTC+4)", en: "Samara (UTC+4)" },
  { value: "Asia/Tbilisi", ru: "Тбилиси (UTC+4)", en: "Tbilisi (UTC+4)" },
  { value: "Asia/Dubai", ru: "Дубай (UTC+4)", en: "Dubai (UTC+4)" },
  { value: "Asia/Yekaterinburg", ru: "Екатеринбург (UTC+5)", en: "Yekaterinburg (UTC+5)" },
  { value: "Asia/Tashkent", ru: "Ташкент (UTC+5)", en: "Tashkent (UTC+5)" },
  { value: "Asia/Almaty", ru: "Алматы (UTC+6)", en: "Almaty (UTC+6)" },
  { value: "Asia/Novosibirsk", ru: "Новосибирск (UTC+7)", en: "Novosibirsk (UTC+7)" },
  { value: "Asia/Bangkok", ru: "Бангкок (UTC+7)", en: "Bangkok (UTC+7)" },
  { value: "Asia/Irkutsk", ru: "Иркутск (UTC+8)", en: "Irkutsk (UTC+8)" },
  { value: "Asia/Shanghai", ru: "Шанхай (UTC+8)", en: "Shanghai (UTC+8)" },
  { value: "Asia/Tokyo", ru: "Токио (UTC+9)", en: "Tokyo (UTC+9)" },
  { value: "Asia/Vladivostok", ru: "Владивосток (UTC+10)", en: "Vladivostok (UTC+10)" },
  { value: "Asia/Kamchatka", ru: "Камчатка (UTC+12)", en: "Kamchatka (UTC+12)" },
  { value: "Australia/Sydney", ru: "Сидней (UTC+10/+11)", en: "Sydney (UTC+10/+11)" },
  { value: "Pacific/Auckland", ru: "Окленд (UTC+12/+13)", en: "Auckland (UTC+12/+13)" },
];

function detectBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

/**
 * NotificationsSheet — bottom sheet with the per-user notification controls:
 *   - master on/off Switch
 *   - preferred time-of-day Select (visible only when notifications are on)
 *
 * On open the component reconciles the browser's IANA timezone with the
 * server-stored one and silently patches /me if they disagree. The user
 * never sees this — they just notice that reminders land in their local
 * morning, not server-UTC morning.
 */
export function NotificationsSheet({ open, onOpenChange }: Props) {
  const { t, i18n } = useTranslation();
  const notificationsEnabled = useSettingsStore(
    (s) => s.settings.notificationsEnabled,
  );
  const notificationTime = useSettingsStore(
    (s) => s.settings.notificationTime,
  );
  const storedTimezone = useSettingsStore((s) => s.settings.timezone);
  const updateSettings = useSettingsStore((s) => s.updateSettings);

  const [savingToggle, setSavingToggle] = useState(false);
  const [savingTime, setSavingTime] = useState(false);
  const [savingTz, setSavingTz] = useState(false);

  // Effective timezone: use stored value, fallback to browser detection
  const effectiveTz = storedTimezone && storedTimezone !== "UTC"
    ? storedTimezone
    : detectBrowserTimezone();

  // Silent timezone sync. Fires once when the sheet becomes visible: if the
  // browser's IANA tz differs from what we have on file, push it. No toast,
  // no UI feedback — this is purely housekeeping. If it fails we just drop
  // it; the worker falls back to UTC.
  // updateSettings is a Zustand action — stable ref, omitted from deps
  // so HMR / strict-mode doesn't trigger a redundant tz round-trip.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!open) return;
    const browserTz = detectBrowserTimezone();
    if (browserTz && browserTz !== storedTimezone) {
      updateSettings({ timezone: browserTz }).catch((err) => {
        console.warn("[notifications] tz sync failed", err);
      });
    }
  }, [open, storedTimezone]);

  const reportError = (err: unknown) => {
    const reason = (err as Error)?.message ?? "unknown";
    toast.error(t("toast.notificationsSaveFailed", { reason }));
  };

  const handleToggle = async (checked: boolean) => {
    if (savingToggle) return;
    hapticSelection();
    setSavingToggle(true);
    try {
      await updateSettings({ notificationsEnabled: checked });
    } catch (err) {
      reportError(err);
    } finally {
      setSavingToggle(false);
    }
  };

  const handleTimeChange = async (value: string) => {
    if (savingTime || value === notificationTime) return;
    hapticSelection();
    setSavingTime(true);
    try {
      await updateSettings({ notificationTime: value });
    } catch (err) {
      reportError(err);
    } finally {
      setSavingTime(false);
    }
  };

  const handleTimezoneChange = async (value: string) => {
    if (savingTz || value === storedTimezone) return;
    hapticSelection();
    setSavingTz(true);
    try {
      await updateSettings({ timezone: value });
      toast.success(t("notifications.timezoneSaved"));
    } catch (err) {
      reportError(err);
    } finally {
      setSavingTz(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="bottom" className="rounded-t-3xl">
        <SheetHeader className="text-left">
          <SheetTitle>{t("notifications.sheetTitle")}</SheetTitle>
          <SheetDescription>{t("notifications.sheetDesc")}</SheetDescription>
        </SheetHeader>

        <div className="mt-4 space-y-2 pb-6">
          {/* Master toggle */}
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
              disabled={savingToggle}
              onCheckedChange={handleToggle}
            />
          </label>

          {/* Time-of-day picker — only meaningful when notifications are on */}
          {notificationsEnabled && (
            <div className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left">
              <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                <Clock className="h-4 w-4" />
              </div>
              <div className="flex-1">
                <p className="text-sm font-semibold">
                  {t("notifications.timeLabel")}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t("notifications.timeHint")}
                </p>
              </div>
              <Select
                value={notificationTime}
                onValueChange={handleTimeChange}
                disabled={savingTime}
              >
                <SelectTrigger className="w-24 border-0 bg-surface-elevated">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TIME_OPTIONS.map((time) => (
                    <SelectItem key={time} value={time}>
                      {time}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Timezone selector */}
          {notificationsEnabled && (
            <div className="bg-surface flex w-full items-center gap-3 rounded-2xl p-4 text-left">
              <div className="bg-surface-elevated flex h-10 w-10 items-center justify-center rounded-xl">
                <Globe className="h-4 w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold">
                  {t("notifications.timezoneLabel")}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {t("notifications.timezoneHint")}
                </p>
              </div>
              <Select
                value={effectiveTz}
                onValueChange={handleTimezoneChange}
                disabled={savingTz}
              >
                <SelectTrigger className="w-36 border-0 bg-surface-elevated text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent className="max-h-60">
                  {TIMEZONE_OPTIONS.map((tz) => (
                    <SelectItem key={tz.value} value={tz.value}>
                      {i18n.language === "ru" ? tz.ru : tz.en}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
