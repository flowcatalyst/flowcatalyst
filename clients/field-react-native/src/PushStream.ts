// PushStream: the push reader (docs/contracts/push.md) for Hermes. Same behaviour and options as field-js's
// PushStream, over the shared push-core.ts; events use addListener (FieldClient style), not EventTarget.
import {
	PushCore,
	type DeniedTopic,
	type PushError,
	type PushMessage,
	type PushStreamEventType,
	type PushStreamOptions,
	type PushStreamState,
	type ResetDetail,
	type SubscribedDetail,
} from "./push-core";

export interface PushStreamEvents {
	subscribed: SubscribedDetail;
	reset: ResetDetail;
	denied: DeniedTopic[];
	error: PushError;
	state: PushStreamState;
}

export type PushStreamEventName = keyof PushStreamEvents;

type Handler = (push: PushMessage) => void;

export class PushStream {
	private readonly core: PushCore;
	private readonly listeners = new Map<PushStreamEventType, Set<(payload: never) => void>>();

	constructor(opts: PushStreamOptions) {
		this.core = new PushCore(opts, (type, detail) => this.dispatch(type, detail));
	}

	/** The id of the last push delivered (or reset seen); persist it and pass it back after a restart. */
	get lastEventId(): string | undefined {
		return this.core.lastEventId;
	}

	get state(): PushStreamState {
		return this.core.state;
	}

	get topics(): readonly string[] {
		return this.core.topics;
	}

	/** Listens for `subscribed`, `reset` (reload your state), `denied`, `error` or `state`. Returns `{ remove() }`. */
	addListener<E extends PushStreamEventName>(event: E, fn: (payload: PushStreamEvents[E]) => void): { remove(): void } {
		let set = this.listeners.get(event);
		if (!set) {
			set = new Set();
			this.listeners.set(event, set);
		}
		const entry = fn as (payload: never) => void;
		set.add(entry);
		return {
			remove: () => {
				set.delete(entry);
			},
		};
	}

	/** Listens for pushes on a topic or pattern (`*` one segment, final `**` the rest). Returns an unsubscribe function. */
	on(pattern: string, fn: Handler): () => void {
		return this.core.on(pattern, fn);
	}

	/** Listens for every push. Returns an unsubscribe function. */
	onAny(fn: Handler): () => void {
		return this.core.onAny(fn);
	}

	start(): void {
		this.core.start();
	}

	stop(): void {
		this.core.stop();
	}

	/** Subscribes again with a new topic set; the last id is kept. */
	setTopics(topics: string[]): void {
		this.core.setTopics(topics);
	}

	private dispatch(type: PushStreamEventType, detail: unknown): void {
		const set = this.listeners.get(type);
		if (!set) return;
		for (const cb of [...set]) {
			try {
				(cb as (x: unknown) => void)(detail);
			} catch {
				// a listener must not break delivery to the others
			}
		}
	}
}
