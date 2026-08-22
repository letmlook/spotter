import { createContext, useContext, useState, useEffect, useMemo, ReactNode } from 'react';

export type ThemeName = 'dark' | 'light';

const STORAGE_KEY = 'spotter-theme';

interface ThemeContextValue {
  theme: ThemeName;
  setTheme: (t: ThemeName) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function readStoredTheme(): ThemeName {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'dark' || v === 'light') return v;
  } catch { /* ignore */ }
  return 'dark';
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeName>(readStoredTheme);

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, theme); } catch { /* ignore */ }
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const value = useMemo<ThemeContextValue>(() => ({
    theme, setTheme: setThemeState,
  }), [theme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}