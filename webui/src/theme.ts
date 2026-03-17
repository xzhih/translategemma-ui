export const appThemeStorageKey = "tg_ui_theme";

export const supportedAppThemes = ["system", "light", "dark"] as const;

export type AppTheme = (typeof supportedAppThemes)[number];
export type ResolvedAppTheme = "light" | "dark";

function canUseDOM() {
  return typeof window !== "undefined" && typeof document !== "undefined";
}

function canUseLocalStorage() {
  return canUseDOM() && typeof window.localStorage?.getItem === "function" && typeof window.localStorage?.setItem === "function";
}

export function normalizeAppTheme(rawTheme: string | null | undefined): AppTheme | null {
  const value = rawTheme?.trim();
  if (!value) {
    return null;
  }
  if (value === "light" || value === "dark" || value === "system") {
    return value;
  }
  return null;
}

export function detectInitialAppTheme(): AppTheme {
  if (!canUseLocalStorage()) {
    return "system";
  }
  return normalizeAppTheme(window.localStorage.getItem(appThemeStorageKey)) ?? "system";
}

export function resolveAppTheme(theme: AppTheme): ResolvedAppTheme {
  if (theme === "light" || theme === "dark") {
    return theme;
  }
  if (!canUseDOM() || typeof window.matchMedia !== "function") {
    return "light";
  }
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export function readAppliedAppTheme(): ResolvedAppTheme {
  if (!canUseDOM()) {
    return "light";
  }
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

export function applyAppTheme(theme: AppTheme): ResolvedAppTheme {
  const resolvedTheme = resolveAppTheme(theme);
  if (!canUseDOM()) {
    return resolvedTheme;
  }
  document.documentElement.dataset.theme = resolvedTheme;
  document.documentElement.dataset.themePreference = theme;
  document.documentElement.style.colorScheme = resolvedTheme;
  return resolvedTheme;
}

export function setAppTheme(theme: AppTheme): ResolvedAppTheme {
  if (canUseLocalStorage()) {
    if (theme === "system") {
      window.localStorage.removeItem(appThemeStorageKey);
    } else {
      window.localStorage.setItem(appThemeStorageKey, theme);
    }
  }
  return applyAppTheme(theme);
}

export function watchSystemTheme(onChange: (theme: ResolvedAppTheme) => void) {
  if (!canUseDOM() || typeof window.matchMedia !== "function") {
    return () => undefined;
  }
  const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  const listener = () => {
    onChange(mediaQuery.matches ? "dark" : "light");
  };
  if (typeof mediaQuery.addEventListener === "function") {
    mediaQuery.addEventListener("change", listener);
    return () => mediaQuery.removeEventListener("change", listener);
  }
  mediaQuery.addListener(listener);
  return () => mediaQuery.removeListener(listener);
}
