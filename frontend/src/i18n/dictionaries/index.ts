// Translation dictionaries for the Spotter GUI. Add new keys to
// each locale file in this directory; missing translations fall
// back to English so a half-translated release still works.
//
// To add a third locale:
//   1. Drop `frontend/src/i18n/dictionaries/<locale>.ts` that
//      exports `<locale>: Record<string, string> = { ... }`.
//   2. Add `'<locale>'` to the Locale union below.
//   3. Register it in the dictionaries record at the bottom.

export type Locale = 'en' | 'zh';

import { en } from './en';
import { zh } from './zh';

export const dictionaries: Record<Locale, Record<string, string>> = {
  en,
  zh,
};
