import { createContext, useContext, useState, useEffect, useMemo, ReactNode } from 'react';
import { dictionaries, Locale } from '../i18n/dictionaries';

const STORAGE_KEY = 'spotter-locale';

interface I18nContextValue {
  locale: Locale;
  setLocale: (l: Locale) => void;
  // Translate a key. Falls back to English, then to the key itself.
  // Supports simple {placeholder} substitution.
  t: (key: string, params?: Record<string, string>) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function readStoredLocale(): Locale {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'en' || v === 'zh') return v;
  } catch { /* ignore */ }
  return 'en';
}

function interpolate(template: string, params?: Record<string, string>): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? `{${k}}`);
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(readStoredLocale);

  useEffect(() => {
    try { localStorage.setItem(STORAGE_KEY, locale); } catch { /* ignore */ }
  }, [locale]);

  const value = useMemo<I18nContextValue>(() => {
    const dict = dictionaries[locale];
    const en = dictionaries.en;
    return {
      locale,
      setLocale: setLocaleState,
      t: (key, params) => interpolate(dict[key] ?? en[key] ?? key, params),
    };
  }, [locale]);

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error('useI18n must be used inside <I18nProvider>');
  return ctx;
}