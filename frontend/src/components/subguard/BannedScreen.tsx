/**
 * Full-screen overlay shown when the API returns 403 account_banned.
 * Hides the entire app UI and displays a localized message.
 */
export function BannedScreen() {
  // Simple locale detection from Telegram WebApp
  const lang = (window as any).Telegram?.WebApp?.initDataUnsafe?.user?.language_code ?? "en";
  const isRu = lang.startsWith("ru");

  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-background">
      <div className="max-w-sm px-6 text-center">
        <div className="mx-auto mb-6 flex h-20 w-20 items-center justify-center rounded-full bg-destructive/10">
          <span className="text-4xl">🚫</span>
        </div>
        <h1 className="text-xl font-bold text-foreground">
          {isRu ? "Доступ ограничен" : "Access Restricted"}
        </h1>
        <p className="mt-3 text-sm text-muted-foreground leading-relaxed">
          {isRu
            ? "Ваш аккаунт заблокирован за нарушение правил сервиса."
            : "Your account has been suspended for violating the terms of service."}
        </p>
      </div>
    </div>
  );
}
