<?php

declare(strict_types=1);

namespace FlowCatalyst\Client;

use FlowCatalyst\Exceptions\FlowCatalystException;

/**
 * @internal Control-flow-only signal used by {@see FlowCatalystClient::request()}
 * to mark a failure as one the retry rule decides on, without changing the
 * type, message, code, or `$data` of the exception that is ultimately
 * thrown to callers once retries are exhausted (or immediately, when the
 * failure is not retryable). Never escapes the client's public API.
 *
 * `$kind` says what the failure tells about the request:
 * - REFUSED: 408 or 429, the request was not processed;
 * - NOT_SENT: the connection was never made, nothing was sent;
 * - GATEWAY: 502, 503 or 504, the request may have been applied;
 * - MAYBE_SENT: a transport failure after the request may have been sent.
 */
final class RetryableFailure extends \RuntimeException
{
    public const REFUSED = 'refused';
    public const NOT_SENT = 'not_sent';
    public const GATEWAY = 'gateway';
    public const MAYBE_SENT = 'maybe_sent';

    public function __construct(
        public readonly FlowCatalystException $exception,
        public readonly string $kind,
    ) {
        parent::__construct($exception->getMessage(), 0, $exception);
    }

    /** The request may have been applied: GATEWAY or MAYBE_SENT. */
    public function outcomeUnknown(): bool
    {
        return $this->kind === self::GATEWAY || $this->kind === self::MAYBE_SENT;
    }
}
