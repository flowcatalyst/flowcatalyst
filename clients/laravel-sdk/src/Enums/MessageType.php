<?php

declare(strict_types=1);

namespace FlowCatalyst\Enums;

enum MessageType: string
{
    /**
     * CloudEvents-compatible event for subscription matching.
     */
    case EVENT = 'EVENT';

    /**
     * Direct webhook dispatch job without subscription matching.
     */
    case DISPATCH_JOB = 'DISPATCH_JOB';

    /**
     * Audit log entry for compliance and debugging.
     */
    case AUDIT_LOG = 'AUDIT_LOG';

    /**
     * Push to clients: the platform's outbox processor sends it to the push
     * gateway (POST /api/push/batch), which streams it to the clients listening
     * on the topic. Written by {@see \FlowCatalyst\Outbox\OutboxManager::createPush()}.
     */
    case PUSH = 'PUSH';
}
