import { createContext, useContext, useState, useEffect, useMemo, ReactNode } from 'react';

export type ThemeMode = 'dark' | 'light' | 'system';
export type ThemeName = 'dark' | 'light';

const STORAGE_KEY = 'spotter-theme';

interface ThemeContextValue {
  mode: ThemeMode;
  theme: ThemeName;
  setMode: (m: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextValue | null>(null);

function readStoredMode(): ThemeMode {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'dark' || v === 'light' || v === 'system') return v;
  } catch { /* ignore */ }
  return 'system';
}

function resolveSystemTheme(): ThemeName {
  if (typeof window === 'undefined' || !window.matchMedia) return 'light';
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

function resolveTheme(mode: ThemeMode): ThemeName {
  return mode === 'system' ? resolveSystemTheme() : mode;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<ThemeMode>(readStoredMode);
  const [theme, setTheme] = useState<ThemeName>(() => resolveTheme(readStoredMode()));

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, mode); } catch { /* ignore */ }
  }, [mode]);

  useEffect(() => {
    const resolved = resolveTheme(mode);
    setTheme(resolved);
    document.documentElement.setAttribute('data-theme', resolved);
  }, [mode]);

  useEffect(() => {
    if (mode !== 'system' || typeof window === 'undefined' || !window.matchMedia) return;
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = () => {
      const resolved = resolveSystemTheme();
      setTheme(resolved);
      document.documentElement.setAttribute('data-theme', resolved);
    };
    mql.addEventListener('change', handler);
    return () => { mql.removeEventListener('change', handler); };
  }, [mode]);

  const value = useMemo<ThemeContextValue>(() => ({
    mode, theme, setMode,
  }), [mode, theme]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme must be used inside <ThemeProvider>');
  return ctx;
}