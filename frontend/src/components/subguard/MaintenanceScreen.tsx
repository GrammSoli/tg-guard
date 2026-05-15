import { useTranslation } from "react-i18next";

/**
 * Full-screen overlay shown when the API returns 503 maintenance_mode.
 * Hides the entire app UI while the backend kill-switch is on.
 * Mirrors BannedScreen's structure for visual consistency.
 *
 * Copy is localized via react-i18next. i18n initialises from the URL
 * ?lang= param / Telegram initData (see lib/i18n.ts) — no API call —
 * so t() resolves correctly even though the request that surfaced this
 * screen was a 503.
 */
export function MaintenanceScreen() {
  const { t } = useTranslation();

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-background">
      <div className="max-w-sm px-6 text-center">
        <div className="mx-auto mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-primary/10">
          {/* Slow spin via the Tailwind-provided `spin` keyframe. */}
          <span
            className="text-4xl"
            style={{ animation: "spin 3s linear infinite", display: "inline-block" }}
          >
            ⚙️
          </span>
        </div>
        <h1 className="text-xl font-bold text-foreground">
          {t("maintenance.title")}
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
          {t("maintenance.subtitle")}
        </p>
      </div>
    </div>
  );
}
