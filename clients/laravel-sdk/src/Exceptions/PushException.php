<?php

declare(strict_types=1);

namespace FlowCatalyst\Exceptions;

/**
 * A push the writer refuses (docs/push-gateway-spec.md section 6.1). The
 * refusal codes are the push contract's (conformance/push/topics.json), the
 * ones the platform's ingest answers with as well; {@see errorCode()} returns
 * one of the CODE_* constants.
 */
final class PushException extends FlowCatalystException
{
    public const CODE_TOPIC_INVALID = 'TOPIC_INVALID';
    public const CODE_TYPE_INVALID = 'TYPE_INVALID';
    public const CODE_TYPE_RESERVED = 'TYPE_RESERVED';
    public const CODE_DATA_INVALID = 'DATA_INVALID';
    public const CODE_DATA_NOT_OBJECT = 'DATA_NOT_OBJECT';
    public const CODE_DATA_TOO_LARGE = 'DATA_TOO_LARGE';
    public const CODE_PUSH_ID_INVALID = 'PUSH_ID_INVALID';
    public const CODE_CLIENT_INVALID = 'CLIENT_INVALID';

    public function __construct(private readonly string $errorCode, string $message)
    {
        parent::__construct("invalid push: {$errorCode}: {$message}");
    }

    /**
     * The refusal code (one of the CODE_* constants).
     */
    public function errorCode(): string
    {
        return $this->errorCode;
    }
}
