<?php

declare(strict_types=1);

namespace FlowCatalyst\Support;

use Odan\Tsid\Tsid;
use Odan\Tsid\TsidFactory;

/**
 * TSID (Time-Sorted ID) Generator
 *
 * Generates 13-character Crockford Base32 encoded IDs that are:
 * - Time-sorted (newer IDs sort after older ones)
 * - URL-safe and case-insensitive
 * - Compatible with FlowCatalyst platform IDs
 */
class TsidGenerator
{
    private static ?TsidFactory $factory = null;

    /**
     * Generate a new TSID string.
     *
     * @return string 13-character Crockford Base32 encoded ID
     */
    public static function generate(): string
    {
        return self::factory()->generate()->toString();
    }

    /**
     * Decode a Crockford Base32 string to a 64-bit value.
     */
    public static function decode(string $tsid): int
    {
        return Tsid::fromString($tsid)->toInt();
    }

    /**
     * Encode a 64-bit value to a TSID string.
     */
    public static function encode(int $value): string
    {
        return Tsid::fromInt($value)->toString();
    }

    /**
     * Validate a TSID string.
     */
    public static function isValid(string $tsid): bool
    {
        if (strlen($tsid) !== 13) {
            return false;
        }

        // Crockford Base32 alphabet (case-insensitive, excludes I, L, O, U)
        return (bool) preg_match('/^[0-9A-HJKMNP-TV-Z]{13}$/i', $tsid);
    }

    /**
     * Extract the timestamp from a TSID.
     */
    public static function getTimestamp(string $tsid): \DateTimeImmutable
    {
        return Tsid::fromString($tsid)->getDateTime();
    }

    private static function factory(): TsidFactory
    {
        return self::$factory ??= new TsidFactory();
    }
}
