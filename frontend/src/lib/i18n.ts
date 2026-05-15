import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import enTranslations from "@/locales/en.json";
import ruTranslations from "@/locales/ru.json";
import { useSettingsStore } from "@/stores/settingsStore";

// ── Resolve initial language ────────────────────────────────────
// Priority: URL ?lang= param > Telegram initData > fallback "en".
// The bot onboarding flow appends ?lang=ru|en to the WebApp URL
// so the user's explicit choice always wins.

function resolveLanguage(): "ru" | "en" {
  // 1. URL parameter (set by the bot's lang: callback)
  try {
    const urlLang = new URLSearchParams(window.location.search).get("lang");
    if (urlLang === "ru" || urlLang === "en") return urlLang;
  } catch {
    /* SSR / non-browser — continue */
  }

  // 2. Telegram initData
  try {
    const tgData = (window as any).Telegram?.WebApp?.initDataUnsafe;
    const code: string = tgData?.user?.language_code || "";
    if (["ru", "be", "uk", "kk"].includes(code)) return "ru";
  } catch {
    /* noop */
  }

  return "en";
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
  fallbackLng: "en",
  interpolation: {
    escapeValue: false, // React already does escaping
  },
});

export default i18n;
