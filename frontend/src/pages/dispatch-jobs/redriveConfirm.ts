/**
 * The double confirmation in front of re-delivering COMPLETED jobs. They were
 * delivered once already; delivering them again can have side effects at the
 * receiver, so the second step asks the user to type REDRIVE and the count.
 * Pure state, so tests/dispatch-jobs-redrive.test.ts can pin the rules.
 */

import { REDRIVE_CONFIRM_WORD } from "@/api/dispatch-jobs";

export type RedrivePhase = "review" | "confirm";

export interface RedriveState {
	phase: RedrivePhase;
	typedWord: string;
	typedCount: string;
}

export const initialRedrive = (): RedriveState => ({ phase: "review", typedWord: "", typedCount: "" });

/** The user chose "Also redrive N COMPLETED jobs": open the second step, nothing typed yet. */
export function startRedrive(): RedriveState {
	return { phase: "confirm", typedWord: "", typedCount: "" };
}

/** Back out of the second step (also what a changed match does: the count they confirmed is stale). */
export function resetRedrive(): RedriveState {
	return initialRedrive();
}

/**
 * The option is offered only to holders of redrive-completed, and only when
 * there is something to redrive.
 */
export function redriveOffered(canRedrive: boolean, completedCount: number): boolean {
	return canRedrive && completedCount > 0;
}

/** Typed digits as a number; thousands separators are tolerated. NaN when not a number. */
export function parseTypedCount(s: string): number {
	const t = s.trim().replace(/[,\s]/g, "");
	return /^\d+$/.test(t) ? Number(t) : Number.NaN;
}

/**
 * The second step may submit only with the word typed exactly and the count
 * retyped correctly. `confirmCount` is the number the request will send
 * (the preview's redriveEligibleCount).
 */
export function redriveReady(s: RedriveState, confirmCount: number): boolean {
	return (
		s.phase === "confirm" &&
		confirmCount > 0 &&
		s.typedWord === REDRIVE_CONFIRM_WORD &&
		parseTypedCount(s.typedCount) === confirmCount
	);
}
