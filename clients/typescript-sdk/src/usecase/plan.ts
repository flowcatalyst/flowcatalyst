/**
 * Plan<E> — a pending change + the domain event it produces, built by an
 * {@link Operation}'s `execute` phase but NOT yet committed. `run` is the only
 * thing that applies a Plan, and it does so inside one transaction (aggregate
 * write + event + optional audit, atomically).
 *
 * Plan is sealed: its discriminant key is a module-private `unique symbol`
 * (the same airtight technique as `src/effect/usecase/seal.ts`), and the only
 * constructors are {@link Plan.save} / {@link Plan.delete} / {@link Plan.emit}.
 * External code can speak the *type* `Plan<E>` but cannot construct one — so an
 * operation cannot reach the database except by returning a Plan and letting
 * `run` apply it. The TypeScript analogue of the Go SDK's sealed `Plan[E]`.
 */

import { Result } from "./result.js";
import { UseCaseError } from "./errors.js";
import { PushError, type CreatePushDto } from "../outbox/create-push-dto.js";
import type { DomainEvent } from "./domain-event.js";
import type { Repo } from "./repo.js";
import type { Aggregate, TxSession } from "./unit-of-work.js";

const PlanKind: unique symbol = Symbol("flowcatalyst.usecase.plan");
type PlanKind = typeof PlanKind;

/** A push to write with the plan: built up front, or from the committed event. */
export type PlannedPush<E extends DomainEvent> =
	| CreatePushDto
	| ((event: E) => CreatePushDto);

interface SavePlan<E extends DomainEvent> {
	readonly pushes?: ReadonlyArray<PlannedPush<E>>;
	readonly [PlanKind]: "save";
	readonly aggregate: Aggregate;
	readonly repo: Repo<Aggregate>;
	readonly event: E;
}

interface DeletePlan<E extends DomainEvent> {
	readonly pushes?: ReadonlyArray<PlannedPush<E>>;
	readonly [PlanKind]: "delete";
	readonly aggregate: Aggregate;
	readonly repo: Repo<Aggregate>;
	readonly event: E;
}

interface EmitPlan<E extends DomainEvent> {
	readonly pushes?: ReadonlyArray<PlannedPush<E>>;
	readonly [PlanKind]: "emit";
	readonly event: E;
}

export type Plan<E extends DomainEvent> =
	| SavePlan<E>
	| DeletePlan<E>
	| EmitPlan<E>;

export const Plan = {
	/** Plan an aggregate upsert with its domain event (create / update). */
	save<A extends Aggregate, E extends DomainEvent>(
		aggregate: A,
		repo: Repo<A>,
		event: E,
	): Plan<E> {
		return {
			[PlanKind]: "save",
			aggregate,
			repo: repo as unknown as Repo<Aggregate>,
			event,
		};
	},

	/** Plan an aggregate deletion with its domain event. */
	delete<A extends Aggregate, E extends DomainEvent>(
		aggregate: A,
		repo: Repo<A>,
		event: E,
	): Plan<E> {
		return {
			[PlanKind]: "delete",
			aggregate,
			repo: repo as unknown as Repo<Aggregate>,
			event,
		};
	},

	/** Plan a domain event with no aggregate change (e.g. UserLoggedIn). */
	emit<E extends DomainEvent>(event: E): Plan<E> {
		return { [PlanKind]: "emit", event };
	},

	/**
	 * Add pushes to a plan: PUSH outbox rows written in the plan's transaction,
	 * after the aggregate and the event, so the clients are told of the change
	 * only if it commits. Each push is a {@link CreatePushDto}, or a function of
	 * the committed event.
	 *
	 *   return usecase.Plan.andPush(
	 *     usecase.Plan.save(order, repo, event),
	 *     CreatePushDto.create(`shop:orders:${order.id}`, "order.updated", { status: "placed" }),
	 *   );
	 *
	 * An invalid push fails the plan (a validation error, the transaction rolls
	 * back) and nothing is written.
	 */
	andPush<E extends DomainEvent>(
		plan: Plan<E>,
		...pushes: Array<PlannedPush<E>>
	): Plan<E> {
		return { ...plan, pushes: [...(plan.pushes ?? []), ...pushes] } as Plan<E>;
	},
};

/** True when `x` is a Plan (used by `run` to tell a Plan from a returned error). */
export function isPlan<E extends DomainEvent = DomainEvent>(
	x: unknown,
): x is Plan<E> {
	return typeof x === "object" && x !== null && PlanKind in x;
}

/**
 * @internal — apply a Plan against the transaction-bound session. Only `run`
 * calls this, inside the transaction the TxRunner owns. Lives here so the
 * `PlanKind` discriminant never has to be exported.
 */
export async function applyPlan<E extends DomainEvent>(
	session: TxSession,
	plan: Plan<E>,
	command: unknown,
): Promise<Result<E>> {
	const committed = await commitPlan(session, plan, command);
	if (!plan.pushes || plan.pushes.length === 0) return committed;
	if (!Result.isSuccess(committed)) return committed;
	// The push rows join the transaction the event was written in. A failure
	// here fails the plan; `run` then rolls the whole transaction back.
	const failure = await writePlannedPushes(session, plan.pushes, committed.value);
	return failure ?? committed;
}

function commitPlan<E extends DomainEvent>(
	session: TxSession,
	plan: Plan<E>,
	command: unknown,
): Promise<Result<E>> {
	switch (plan[PlanKind]) {
		case "save":
			return session.commit(plan.event, command, () =>
				session.withTx((tx) => plan.repo.persist(plan.aggregate, tx)),
			);
		case "delete":
			return session.commit(plan.event, command, () =>
				session.withTx((tx) => plan.repo.delete(plan.aggregate, tx)),
			);
		case "emit":
			return session.emitEvent(plan.event, command);
	}
}

async function writePlannedPushes<E extends DomainEvent>(
	session: TxSession,
	pushes: ReadonlyArray<PlannedPush<E>>,
	committed: E,
): Promise<Result<E> | null> {
	if (!session.emitPush) {
		return Result.failure<E>(
			UseCaseError.infrastructure(
				"PUSH_UNSUPPORTED",
				"this unit of work cannot write pushes (no emitPush)",
			),
		);
	}
	try {
		for (const p of pushes) {
			await session.emitPush(typeof p === "function" ? p(committed) : p);
		}
		return null;
	} catch (err) {
		if (err instanceof PushError) {
			return Result.failure<E>(
				UseCaseError.validation(err.code, err.message),
			);
		}
		const message = err instanceof Error ? err.message : String(err);
		return Result.failure<E>(
			UseCaseError.infrastructure("PUSH_FAILED", message, { cause: message }),
		);
	}
}
