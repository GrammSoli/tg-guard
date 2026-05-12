import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import enTranslations from "@/locales/en.json";
import ruTranslations from "@/locales/ru.json";

// Read from Telegram initData
const tgData = (window as any).Telegram?.WebApp?.initDataUnsafe;
let tgLang = tgData?.user?.language_code || "en";

// Map aliases to 'ru'
if (["ru", "be", "uk", "kk"].includes(tgLang)) {
  tgLang = "ru";
} else if (tgLang !== "ru") {
  tgLang = "en"; // Fallback everything else to English
}

i18n
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: enTranslations },
      ru: { translation: ruTranslations },
    },
    lng: tgLang,
    fallbackLng: "en",
    interpolation: {
      escapeValue: false, // React already does escaping
    },
  });

export default i18n;
