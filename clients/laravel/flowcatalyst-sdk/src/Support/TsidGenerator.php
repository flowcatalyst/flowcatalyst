<?php

declare(strict_types=1);

namespace FlowCatalyst\Support;

/**
 * TSID (Time-Sorted ID) Generator
 *
 * Generates 13-character Crockford Base32 encoded IDs that are:
 * - Time-sorted (newer IDs sort after older ones)
 * - URL-safe and case-insensitive
 * - Compatible with FlowCatalyst platform IDs
 *
 * Format: 42 bits timestamp (milliseconds) + 22 bits random
 */
class TsidGenerator
{
    /**
     * Crockford Base32 alphabet (excludes I, L, O, U to avoid confusion).
     */
    private const ALPHABET = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

    /**
     * Custom epoch: 2020-01-01T00:00:00Z (reduces timestamp bits needed).
     */
    private const EPOCH = 1577836800000;

    /**
     * Number of random bits.
     */
    private const RANDOM_BITS = 22;

    /**
     * Counter for same-millisecond generation.
     */
    private static int $counter = 0;

    /**
     * Last generation timestamp.
     */
    private static int $lastTimestamp = 0;

    /**
     * Generate a new TSID string.
     *
     * @return string 13-character Crockford Base32 encoded ID
     */
    public static function generate(): string
    {
        $timestamp = self::currentTimeMillis() - self::EPOCH;

        // Handle same-millisecond generation with counter
        if ($timestamp === self::$lastTimestamp) {
            self::$counter++;
            if (self::$counter >= (1 << self::RANDOM_BITS)) {
                // Counter overflow, wait for next millisecond
                while (($timestamp = self::currentTimeMillis() - self::EPOCH) === self::$lastTimestamp) {
                    usleep(100);
                }
                self::$counter = self::randomBits();
            }
        } else {
            self::$counter = self::randomBits();
            self::$lastTimestamp = $timestamp;
        }

        // Combine timestamp and random/counter bits
        // 42 bits for timestamp, 22 bits for random/counter
        $value = ($timestamp << self::RANDOM_BITS) | (self::$counter & ((1 << self::RANDOM_BITS) - 1));

        return self::encode($value);
    }

    /**
     * Encode a 64-bit value to Crockford Base32.
     */
    private static function encode(int $value): string
    {
        $result = '';

        // Encode 13 characters (65 bits capacity, we use 64)
        for ($i = 0; $i < 13; $i++) {
            $result = self::ALPHABET[$value & 0x1F] . $result;
            $value >>= 5;
        }

        return $result;
    }

    /**
     * Decode a Crockford Base32 string to a 64-bit value.
     */
    public static function decode(string $tsid): int
    {
        $tsid = strtoupper($tsid);
        $value = 0;

        for ($i = 0; $i < strlen($tsid); $i++) {
            $char = $tsid[$i];
            $index = strpos(self::ALPHABET, $char);

            if ($index === false) {
                // Handle common substitutions
                $char = match ($char) {
                    'I', 'L' => '1',
                    'O' => '0',
                    default => $char,
                };
                $index = strpos(self::ALPHABET, $char);
            }

            if ($index === false) {
                throw new \InvalidArgumentException("Invalid TSID character: {$char}");
            }

            $value = ($value << 5) | $index;
        }

        return $value;
    }

    /**
     * Get current time in milliseconds.
     */
    private static function currentTimeMillis(): int
    {
        return (int) (microtime(true) * 1000);
    }

    /**
     * Generate random bits for the counter portion.
     */
    private static function randomBits(): int
    {
        return random_int(0, (1 << self::RANDOM_BITS) - 1);
    }

    /**
     * Validate a TSID string.
     */
    public static function isValid(string $tsid): bool
    {
        if (strlen($tsid) !== 13) {
            return false;
        }

        $tsid = strtoupper($tsid);
        for ($i = 0; $i < 13; $i++) {
            $char = $tsid[$i];
            if (strpos(self::ALPHABET, $char) === false) {
                // Check for common substitutions
                if (!in_array($char, ['I', 'L', 'O', 'U'], true)) {
                    return false;
                }
            }
        }

        return true;
    }

    /**
     * Extract the timestamp from a TSID.
     *
     * @return \DateTimeImmutable
     */
    public static function getTimestamp(string $tsid): \DateTimeImmutable
    {
        $value = self::decode($tsid);
        $timestamp = ($value >> self::RANDOM_BITS) + self::EPOCH;

        return \DateTimeImmutable::createFromFormat('U.u', sprintf('%.3f', $timestamp / 1000));
    }
}
