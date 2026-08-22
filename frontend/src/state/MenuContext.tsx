import { createContext, useContext, useState, useCallback, useMemo, ReactNode } from 'react';

export type ModalKind = null | 'setup-guide' | 'add-device' | 'about';

interface MenuContextValue {
  modal: ModalKind;
  openModal: (k: ModalKind) => void;
  closeModal: () => void;
  // Triggers a one-click scan; the App routes this through the
  // scanner binding. Surfaced here so menu items can fire it.
  triggerScan: () => Promise<void>;
  setTriggerScan: (fn: () => Promise<void>) => void;
}

const MenuContext = createContext<MenuContextValue | null>(null);

export function MenuProvider({ children }: { children: ReactNode }) {
  const [modal, setModal] = useState<ModalKind>(null);
  const [scanFn, setScanFn] = useState<() => Promise<void>>(() => async () => {});

  const value = useMemo<MenuContextValue>(() => ({
    modal,
    openModal: setModal,
    closeModal: () => setModal(null),
    triggerScan: scanFn,
    setTriggerScan: (fn) => setScanFn(() => fn),
  }), [modal, scanFn]);

  return <MenuContext.Provider value={value}>{children}</MenuContext.Provider>;
}

export function useMenu(): MenuContextValue {
  const ctx = useContext(MenuContext);
  if (!ctx) throw new Error('useMenu must be used inside <MenuProvider>');
  return ctx;
}

// suppress unused-imports warning for useCallback (kept for future use)
void useCallback;