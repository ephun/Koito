import {
  createContext,
  useEffect,
  useState,
  useCallback,
  type ReactNode,
} from "react";
import { type Theme, themes } from "~/styles/themes.css";
import { themeVars } from "~/styles/vars.css";
import { useAppContext } from "./AppProvider";

interface ThemeContextValue {
  themeName: string;
  theme: Theme;
  setTheme: (theme: string) => void;
  resetTheme: () => void;
  setCustomTheme: (theme: Theme) => void;
  getCustomTheme: () => Theme | undefined;
}

const ThemeContext = createContext<ThemeContextValue | undefined>(undefined);

function readPreference(key: string): string | null {
  try { return localStorage.getItem(key); } catch { return null; }
}
function writePreference(key: string, value: string | null) {
  try { if (value === null) localStorage.removeItem(key); else localStorage.setItem(key, value); } catch { /* The current session still works when storage is unavailable. */ }
}

function toKebabCase(str: string) {
  return str.replace(/[A-Z]/g, (m) => "-" + m.toLowerCase());
}

function applyCustomThemeVars(theme: Theme) {
  const root = document.documentElement;
  for (const [key, value] of Object.entries(theme)) {
    if (key === "name") continue;
    root.style.setProperty(`--color-${toKebabCase(key)}`, value);
  }
}

function clearCustomThemeVars() {
  for (const cssVar of Object.values(themeVars)) {
    document.documentElement.style.removeProperty(cssVar);
  }
}

function getStoredCustomTheme(): Theme | undefined {
  const themeStr = readPreference("custom-theme");
  if (!themeStr) return undefined;
  try {
    const parsed = JSON.parse(themeStr);
    if (!parsed || typeof parsed !== "object" || !Object.keys(themes.yuu).every(key => typeof parsed[key] === "string")) return undefined;
    const { name, ...theme } = parsed;
    return theme as Theme;
  } catch {
    return undefined;
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  let defaultTheme = useAppContext().defaultTheme;
  let initialTheme = readPreference("theme") ?? defaultTheme;
  const initialCustomTheme = initialTheme === "custom" ? getStoredCustomTheme() : undefined;
  const [themeName, setThemeName] = useState(
    initialCustomTheme || Object.hasOwn(themes, initialTheme) ? initialTheme : defaultTheme,
  );
  const [currentTheme, setCurrentTheme] = useState<Theme>(() => {
    if (initialTheme === "custom") {
      return initialCustomTheme || themes[defaultTheme];
    }
    return Object.hasOwn(themes, initialTheme) ? themes[initialTheme] : themes[defaultTheme];
  });

  useEffect(() => {
    const stored = readPreference("theme");
    if (!stored || (!Object.hasOwn(themes, stored) && !(stored === "custom" && getStoredCustomTheme()))) {
      setThemeName(defaultTheme);
      setCurrentTheme(themes[defaultTheme]);
    }
  }, [defaultTheme]);

  const setTheme = (newThemeName: string) => {
    setThemeName(newThemeName);
    if (newThemeName === "custom") {
      const customTheme = getStoredCustomTheme();
      if (customTheme) {
        setCurrentTheme(customTheme);
      } else {
        // Fallback to default theme if no custom theme found
        setThemeName(defaultTheme);
        setCurrentTheme(themes[defaultTheme]);
      }
    } else {
      const foundTheme = Object.hasOwn(themes, newThemeName) ? themes[newThemeName] : undefined;
      if (foundTheme) {
        writePreference("theme", newThemeName);
        setCurrentTheme(foundTheme);
      } else {
        setTheme(defaultTheme);
      }
    }
  };

  const resetTheme = () => {
    setThemeName(defaultTheme);
    writePreference("theme", null);
    setCurrentTheme(themes[defaultTheme]);
  };

  const setCustomTheme = useCallback((customTheme: Theme) => {
    writePreference("custom-theme", JSON.stringify(customTheme));
    applyCustomThemeVars(customTheme);
    setThemeName("custom");
    writePreference("theme", "custom");
    setCurrentTheme(customTheme);
  }, []);

  const getCustomTheme = (): Theme | undefined => {
    return getStoredCustomTheme();
  };

  useEffect(() => {
    const root = document.documentElement;

    root.setAttribute("data-theme", themeName);
    writePreference("bgcolor", currentTheme.bg);

    if (themeName === "custom") {
      applyCustomThemeVars(currentTheme);
    } else {
      clearCustomThemeVars();
    }
  }, [themeName, currentTheme]);

  return (
    <ThemeContext.Provider
      value={{
        themeName,
        theme: currentTheme,
        setTheme,
        resetTheme,
        setCustomTheme,
        getCustomTheme,
      }}
    >
      {children}
    </ThemeContext.Provider>
  );
}

export { ThemeContext };
