/**
 * Event bus for global error/notification handling.
 *
 * This allows non-Vue code (like API interceptors) to emit notifications
 * that Vue components can listen to and display via PrimeVue Toast.
 */

export type NotificationSeverity = 'success' | 'info' | 'warn' | 'error';

export interface Notification {
  severity: NotificationSeverity;
  summary: string;
  detail?: string;
  life?: number;
}

type NotificationHandler = (notification: Notification) => void;

const handlers: Set<NotificationHandler> = new Set();

/**
 * Subscribe to notifications.
 * Returns an unsubscribe function.
 */
export function onNotification(handler: NotificationHandler): () => void {
  handlers.add(handler);
  return () => handlers.delete(handler);
}

/**
 * Emit a notification to all subscribers.
 */
export function notify(notification: Notification): void {
  handlers.forEach(handler => handler(notification));
}

/**
 * Convenience methods for common notification types.
 */
export const toast = {
  success(summary: string, detail?: string) {
    notify({ severity: 'success', summary, detail, life: 3000 });
  },
  info(summary: string, detail?: string) {
    notify({ severity: 'info', summary, detail, life: 5000 });
  },
  warn(summary: string, detail?: string) {
    notify({ severity: 'warn', summary, detail, life: 5000 });
  },
  error(summary: string, detail?: string) {
    notify({ severity: 'error', summary, detail, life: 5000 });
  },
};
