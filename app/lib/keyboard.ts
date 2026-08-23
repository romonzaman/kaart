import { useEffect } from 'react';
import { Platform } from 'react-native';

export type KeyHandler = (event: KeyboardEvent) => void;

/**
 * Binds document-level key handlers on web and does nothing everywhere else.
 *
 * The review loop has to be completable without leaving the keyboard, and the
 * card editor saves on Cmd/Ctrl+Enter. On iOS this is inert — there is no
 * keyboard to bind and no `window` to bind it to.
 */
export function useKeyboardShortcuts(handler: KeyHandler, enabled = true): void {
  useEffect(() => {
    if (Platform.OS !== 'web' || !enabled) return;
    if (typeof window === 'undefined') return;

    const listener = (event: KeyboardEvent) => handler(event);
    window.addEventListener('keydown', listener);
    return () => window.removeEventListener('keydown', listener);
  }, [handler, enabled]);
}

/**
 * True when the key event came from a text field, so a global shortcut should
 * step aside. Typing "3" into the front of a card must not rate a review.
 */
export function isTypingTarget(event: KeyboardEvent): boolean {
  const target = event.target as { tagName?: string; isContentEditable?: boolean } | null;
  if (target === null) return false;
  const tag = target.tagName?.toLowerCase();
  return tag === 'input' || tag === 'textarea' || target.isContentEditable === true;
}
