import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import enTranslation from "./locales/en/translation.json";
import deDETranslation from "./locales/de-DE/translation.json";
import esTranslation from "./locales/es/translation.json";
import frTranslation from "./locales/fr/translation.json";
import jaTranslation from "./locales/ja/translation.json";
import koTranslation from "./locales/ko/translation.json";
import zhCNTranslation from "./locales/zh-CN/translation.json";

export const appLocaleStorageKey = "tg_ui_locale";
export const supportedAppLocales = ["en", "zh-CN", "ja", "ko", "de-DE", "fr", "es"] as const;

export type AppLocale = (typeof supportedAppLocales)[number];

const resources = {
  en: {
    translation: enTranslation,
  },
  "zh-CN": {
    translation: zhCNTranslation,
  },
  ja: {
    translation: jaTranslation,
  },
  ko: {
    translation: koTranslation,
  },
  "de-DE": {
    translation: deDETranslation,
  },
  fr: {
    translation: frTranslation,
  },
  es: {
    translation: esTranslation,
  },
} as const;

function canUseDOM() {
  return typeof window !== "undefined" && typeof document !== "undefined";
}

function canUseLocalStorage() {
  return canUseDOM() && typeof window.localStorage?.getItem === "function" && typeof window.localStorage?.setItem === "function";
}

export function normalizeAppLocale(rawLocale: string | null | undefined): AppLocale | null {
  const value = rawLocale?.trim();
  if (!value) {
    return null;
  }
  if (value === "en" || value.startsWith("en-")) {
    return "en";
  }
  if (value === "zh-CN" || value === "zh" || value.startsWith("zh-")) {
    return "zh-CN";
  }
  if (value === "ja" || value.startsWith("ja-")) {
    return "ja";
  }
  if (value === "ko" || value.startsWith("ko-")) {
    return "ko";
  }
  if (value === "de-DE" || value === "de" || value.startsWith("de-")) {
    return "de-DE";
  }
  if (value === "fr" || value.startsWith("fr-")) {
    return "fr";
  }
  if (value === "es" || value.startsWith("es-")) {
    return "es";
  }
  return null;
}

function detectInitialLocale(): AppLocale {
  if (!canUseDOM()) {
    return "en";
  }

  const params = new URLSearchParams(window.location.search);
  const fromQuery = normalizeAppLocale(params.get("lang"));
  if (fromQuery) {
    if (canUseLocalStorage()) {
      window.localStorage.setItem(appLocaleStorageKey, fromQuery);
    }
    return fromQuery;
  }

  if (canUseLocalStorage()) {
    const fromStorage = normalizeAppLocale(window.localStorage.getItem(appLocaleStorageKey));
    if (fromStorage) {
      return fromStorage;
    }
  }

  for (const candidate of navigator.languages) {
    const normalized = normalizeAppLocale(candidate);
    if (normalized) {
      return normalized;
    }
  }

  return normalizeAppLocale(navigator.language) ?? "en";
}

function applyDocumentLocale(locale: AppLocale) {
  if (!canUseDOM()) {
    return;
  }
  document.documentElement.lang = locale;
}

void i18n.use(initReactI18next).init({
  resources,
  lng: detectInitialLocale(),
  fallbackLng: "en",
  interpolation: {
    escapeValue: false,
  },
});

applyDocumentLocale((i18n.resolvedLanguage as AppLocale | undefined) ?? "en");

export async function setAppLocale(locale: AppLocale) {
  if (canUseLocalStorage()) {
    window.localStorage.setItem(appLocaleStorageKey, locale);
  }
  await i18n.changeLanguage(locale);
  applyDocumentLocale(locale);
}

export default i18n;
