<?php

declare(strict_types=1);

namespace FlowCatalyst\UseCase;

use FlowCatalyst\Outbox\DTOs\CreatePushDto;

/**
 * A {@see UnitOfWork} that can also write PUSH outbox rows (see
 * {@see Plan::andPush()}). A separate contract so existing UnitOfWork
 * implementations keep working; {@see OutboxUnitOfWork} implements it.
 */
interface PushesUnitOfWork extends UnitOfWork
{
    /**
     * Write a PUSH outbox row in the current transaction. The push's client
     * code defaults to the unit of work's.
     *
     * @throws \FlowCatalyst\Exceptions\PushException when the push is invalid
     */
    public function emitPush(CreatePushDto $push): void;
}
