/**
 * Full-screen overlay shown when the API returns 503 maintenance_mode.
 * Hides the entire app UI while the backend kill-switch is on.
 * Mirrors BannedScreen's structure for visual consistency.
 */
export function MaintenanceScreen() {
  // Simple locale detection from Telegram WebApp — the settings store
  // may not have loaded (its /me call is exactly what got 503'd).
  const lang =
    (window as any).Telegram?.WebApp?.initDataUnsafe?.user?.language_code ?? "en";
  const isRu = lang.startsWith("ru");

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
          {isRu ? "Технические работы" : "Under Maintenance"}
        </h1>
        <p className="mt-3 text-sm leading-relaxed text-muted-foreground">
          {isRu
            ? "Прикручиваем новые фичи, скоро вернёмся ☕️"
            : "Bolting on new features — back in a moment ☕️"}
        </p>
      </div>
    </div>
  );
}
