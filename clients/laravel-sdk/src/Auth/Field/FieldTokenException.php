<?php

declare(strict_types=1);

namespace FlowCatalyst\Auth\Field;

/**
 * A refused field credential. {@see self::$reason} is one of the REASON_*
 * constants, the same stable strings the shared conformance fixtures
 * (`api/field-fixtures`) and the Go SDK (`auth.FieldReason*`) use.
 *
 * Key-fetch / discovery failures are NOT this exception: they surface as the
 * underlying \Throwable so callers can answer 503 rather than 401.
 */
final class FieldTokenException extends \RuntimeException
{
    public const REASON_MALFORMED = 'malformed';
    public const REASON_WRONG_TYP = 'wrong_typ';
    public const REASON_WRONG_TOKEN_USE = 'wrong_token_use';
    public const REASON_WRONG_AUD = 'wrong_aud';
    public const REASON_EXPIRED = 'expired';
    public const REASON_INVALID_TOKEN = 'invalid_token';
    public const REASON_PCR = 'pcr';
    public const REASON_MISSING_PROOF = 'missing_proof';
    public const REASON_INVALID_PROOF = 'invalid_proof';
    public const REASON_JKT_MISMATCH = 'jkt_mismatch';
    public const REASON_HTM_MISMATCH = 'htm_mismatch';
    public const REASON_HTU_MISMATCH = 'htu_mismatch';
    public const REASON_ATH_MISMATCH = 'ath_mismatch';
    public const REASON_STALE_PROOF = 'stale_proof';

    public function __construct(public readonly string $reason, public readonly string $detail = '')
    {
        parent::__construct(
            $detail === ''
                ? "field credential refused: {$reason}"
                : "field credential refused: {$reason}: {$detail}",
        );
    }
}
