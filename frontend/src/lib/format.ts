export function formatCurrency(amount: number, currency: string, locale = "en-US") {
  const safe = Number.isFinite(amount) ? amount : 0;
  return new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(safe);
}

export function formatDate(iso: string, locale = "en-US") {
  return new Intl.DateTimeFormat(locale, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(new Date(iso));
}

const LOCALE_MAP: Record<string, string> = { ru: "ru-RU", en: "en-US" };

export function localeFor(lang: string): string {
  return LOCALE_MAP[lang] ?? "en-US";
}
