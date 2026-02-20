/**
 * EventType Status
 */

export type EventTypeStatus = "CURRENT" | "ARCHIVED";

export const EventTypeStatus = {
	CURRENT: "CURRENT" as const,
	ARCHIVED: "ARCHIVED" as const,
} as const;
