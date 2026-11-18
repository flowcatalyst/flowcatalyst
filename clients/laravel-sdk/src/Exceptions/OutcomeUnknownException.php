<?php

declare(strict_types=1);

namespace FlowCatalyst\Exceptions;

/**
 * A request that is not safe to send twice may have been applied by the
 * platform, and its answer was lost: a gateway answered 502, 503 or 504, or
 * the connection failed or timed out after the request was sent. The client
 * did not send it again ({@see \FlowCatalyst\Client\FlowCatalystClient::request()}).
 *
 * Unlike a refusal (403, 404, 409, 422, ...) the platform's state is unknown:
 * read it before deciding to send the request again (after a provisioning,
 * `admin()->applications()->getServiceAccount($id)`).
 *
 * It is a {@see FlowCatalystException}, so code that caught that still does.
 * `getCode()` is the gateway's status, or 0 after a transport failure, whose
 * exception is `getPrevious()`; `getContext()` is the gateway's decoded body.
 */
final class OutcomeUnknownException extends FlowCatalystException
{
    public function __construct(
        public readonly string $method,
        public readonly string $endpoint,
        public readonly ?int $status = null,
        ?array $body = null,
        ?\Throwable $previous = null,
    ) {
        $happened = $status !== null
            ? "answered {$status}"
            : 'failed after it was sent (' . ($previous?->getMessage() ?? 'transport error') . ')';
        parent::__construct(
            "{$method} {$endpoint} {$happened}: the request may have been applied and was not repeated",
            $status ?? 0,
            $previous,
            $body,
        );
    }
}
