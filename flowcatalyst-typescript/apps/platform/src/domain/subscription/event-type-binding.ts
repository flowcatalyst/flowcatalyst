/**
 * Event Type Binding
 *
 * Links a subscription to an event type with optional spec version.
 */

export interface EventTypeBinding {
	readonly eventTypeId: string | null;
	readonly eventTypeCode: string;
	readonly specVersion: string | null;
}
