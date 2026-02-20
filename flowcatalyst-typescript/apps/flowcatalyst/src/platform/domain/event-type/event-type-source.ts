/**
 * EventType Source
 *
 * Indicates how an event type was created:
 * - CODE: Platform-defined event types
 * - API: Created via SDK/API sync
 * - UI: Created via admin UI
 */

export type EventTypeSource = "CODE" | "API" | "UI";

export const EventTypeSource = {
	CODE: "CODE" as const,
	API: "API" as const,
	UI: "UI" as const,
} as const;
