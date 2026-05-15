import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import enTranslations from "@/locales/en.json";
import ruTranslations from "@/locales/ru.json";
import { useSettingsStore } from "@/stores/settingsStore";

// ── Resolve initial language ────────────────────────────────────
// Priority: URL ?lang= param > Telegram client language > fallback "ru".
//
// This runs SYNCHRONOUSLY at module load — before the app renders and
// without any API call. That matters: when the backend is in
// maintenance mode every /api request 503s, so the language MUST be
// resolved from sources that don't depend on the server (URL params,
// the Telegram SDK). Otherwise MaintenanceScreen would render before
// the (never-arriving) /me response and fall back to the wrong locale.
//
// telegram-web-app.js is a plain <script> in index.html (no defer), so
// it executes before this deferred ES-module bundle — window.Telegram
// .WebApp.initDataUnsafe is already populated when resolveLanguage runs.

function resolveLanguage(): "ru" | "en" {
  // 1. Explicit choice — the bot's lang: callback appends ?lang=ru|en.
  try {
    const urlLang = new URLSearchParams(window.location.search).get("lang");
    if (urlLang === "ru" || urlLang === "en") return urlLang;
  } catch {
    /* SSR / non-browser — continue */
  }

  // 2. Telegram client language. language_code may arrive as a short
  //    code ("en") OR a full IETF tag ("en-US", "ru-RU") depending on
  //    the client — match by PREFIX so both forms resolve. English is
  //    detected explicitly (not by elimination) so an en* user is never
  //    swept into the ru fallback.
  try {
    const code = String(
      (window as any).Telegram?.WebApp?.initDataUnsafe?.user?.language_code ?? "",
    ).toLowerCase();
    if (code.startsWith("en")) return "en";
    if (
      code.startsWith("ru") ||
      code.startsWith("be") ||
      code.startsWith("uk") ||
      code.startsWith("kk")
    ) {
      return "ru";
    }
  } catch {
    /* noop */
  }

  // 3. Fallback — the product's primary audience is Russian-speaking.
  return "ru";
}

const finalLang = resolveLanguage();

// Eagerly sync with the settings store so components reading
// locale from Zustand see the correct value before fetchProfile.
useSettingsStore.getState().settings.locale = finalLang;

i18n.use(initReactI18next).init({
  resources: {
    en: { translation: enTranslations },
    ru: { translation: ruTranslations },
  },
  lng: finalLang,
  fallbackLng: "ru",
  interpolation: {
    escapeValue: false, // React already does escaping
  },
  react: {
    // Disable Suspense so useTranslation() returns synchronously. The
    // resources above are inline (no async backend), so t() is usable
    // the instant init() is called — there's nothing to suspend FOR,
    // and a suspense throw with no boundary above MaintenanceScreen
    // (which renders outside QueryClientProvider) would blank the app.
    useSuspense: false,
  },
});

export default i18n;
