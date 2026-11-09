<?php

declare(strict_types=1);

namespace FlowCatalyst\UseCase;

use FlowCatalyst\Exceptions\PushException;
use FlowCatalyst\Outbox\DTOs\CreatePushDto;

/**
 * Plan — a pending change + the domain event it produces, built by an
 * {@link Operation}'s `execute` phase but NOT yet committed. {@link Runner} is
 * the only thing that applies a Plan, and it does so inside one transaction
 * (aggregate write + event + optional audit, atomically).
 *
 * Plan is sealed: its constructor is private, so the only ways to build one are
 * {@link Plan::save()} / {@link Plan::delete()} / {@link Plan::emit()}. An
 * operation therefore cannot reach the database except by returning a Plan and
 * letting the Runner apply it. (PHP has no module-private visibility, so the
 * seal is convention-level — but the structure carries the invariant: `execute`
 * returns a Plan and only the Runner applies it.) The PHP analogue of the Go
 * SDK's sealed `Plan[E]`.
 *
 * @template TEvent of DomainEvent
 */
final class Plan
{
    private const KIND_SAVE = 'save';
    private const KIND_DELETE = 'delete';
    private const KIND_EMIT = 'emit';

    private function __construct(
        private readonly string $kind,
        private readonly DomainEvent $event,
        private readonly ?Aggregate $aggregate = null,
        private readonly ?Repo $repo = null,
        /** @var list<CreatePushDto|callable(DomainEvent):CreatePushDto> */
        private readonly array $pushes = [],
    ) {}

    /**
     * Plan an aggregate upsert with its domain event (create / update).
     *
     * @template T of DomainEvent
     * @param T $event
     * @return self<T>
     */
    public static function save(Aggregate $aggregate, Repo $repo, DomainEvent $event): self
    {
        return new self(self::KIND_SAVE, $event, $aggregate, $repo);
    }

    /**
     * Plan an aggregate deletion with its domain event.
     *
     * @template T of DomainEvent
     * @param T $event
     * @return self<T>
     */
    public static function delete(Aggregate $aggregate, Repo $repo, DomainEvent $event): self
    {
        return new self(self::KIND_DELETE, $event, $aggregate, $repo);
    }

    /**
     * Plan a domain event with no aggregate change (e.g. UserLoggedIn).
     *
     * @template T of DomainEvent
     * @param T $event
     * @return self<T>
     */
    public static function emit(DomainEvent $event): self
    {
        return new self(self::KIND_EMIT, $event);
    }

    /**
     * Add pushes to a plan: PUSH outbox rows written in the plan's transaction,
     * after the aggregate and the event, so the clients are told of the change
     * only if it commits. Each push is a {@see CreatePushDto}, or a callable of
     * the committed event returning one.
     *
     *     return Plan::save($order, $repo, $event)->andPush(
     *         CreatePushDto::create("shop:orders:{$order->id()}", 'order.updated', ['status' => 'placed']),
     *         fn (OrderPlaced $e) => CreatePushDto::create('shop:board', 'refresh', ['eventId' => $e->eventId()])->withUngrouped(),
     *     );
     *
     * An invalid push fails the plan (a validation error carrying the push's
     * code) and the transaction rolls back: nothing is written. The unit of
     * work must implement {@see PushesUnitOfWork} ({@see OutboxUnitOfWork} does).
     *
     * @param CreatePushDto|callable(DomainEvent):CreatePushDto ...$pushes
     * @return self<TEvent>
     */
    public function andPush(CreatePushDto|callable ...$pushes): self
    {
        return new self(
            $this->kind,
            $this->event,
            $this->aggregate,
            $this->repo,
            [...$this->pushes, ...array_values($pushes)],
        );
    }

    /**
     * Apply the plan against the unit of work. Only {@link Runner} calls this,
     * inside the transaction it owns — the aggregate write (via the Repo) runs
     * before the event commit so both land in the same transaction.
     *
     * @return Result<TEvent>
     *
     * @internal
     */
    public function apply(UnitOfWork $uow, mixed $command): Result
    {
        $committed = $this->commitPlan($uow, $command);
        if ($this->pushes === [] || $committed->isFailure()) {
            return $committed;
        }

        // The push rows join the transaction the event was written in. A failure
        // here fails the plan; the Runner then rolls the whole transaction back.
        if (!$uow instanceof PushesUnitOfWork) {
            return Result::failure(UseCaseError::infrastructure(
                'PUSH_UNSUPPORTED',
                'this unit of work cannot write pushes (it does not implement PushesUnitOfWork)',
            ));
        }
        try {
            foreach ($this->pushes as $push) {
                $uow->emitPush($push instanceof CreatePushDto ? $push : $push($this->event));
            }
        } catch (PushException $e) {
            return Result::failure(UseCaseError::validation($e->errorCode(), $e->getMessage()));
        } catch (\Throwable $e) {
            return Result::failure(UseCaseError::infrastructure('PUSH_FAILED', $e->getMessage(), ['cause' => $e::class]));
        }

        return $committed;
    }

    private function commitPlan(UnitOfWork $uow, mixed $command): Result
    {
        switch ($this->kind) {
            case self::KIND_SAVE:
                /** @var Aggregate $agg */
                $agg = $this->aggregate;
                /** @var Repo $repo */
                $repo = $this->repo;
                $repo->persist($agg);
                return $uow->commit($this->event, $command);
            case self::KIND_DELETE:
                /** @var Aggregate $agg */
                $agg = $this->aggregate;
                /** @var Repo $repo */
                $repo = $this->repo;
                $repo->delete($agg);
                return $uow->commit($this->event, $command);
            default: // KIND_EMIT
                return $uow->emitEvent($this->event, $command);
        }
    }
}
