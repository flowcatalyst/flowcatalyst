<?php

declare(strict_types=1);

namespace FlowCatalyst\Tests\Support;

/**
 * The checks every verifier of a signed token runs (docs/wire-contract.md,
 * "Token spelling"): the issued value spelled other ways that a lenient
 * decoder reads as the same bytes, and the issued value changed in ways that
 * certainly alter its content or its signature bytes. Each case is checked to
 * differ from the issued value, and each re-spelling to decode to the issued
 * signature, so no case passes by chance.
 */
final class Respellings
{
    public const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

    /** Spare low bits of the last character of an unpadded base64url string of `$n` characters. */
    public static function spareBits(int $n): int
    {
        return match ($n % 4) {
            2 => 4,
            3 => 2,
            default => 0,
        };
    }

    /** @return list<string> every other last character of `$seg` with the same significant bits */
    public static function nonCanonicalSpellings(string $seg): array
    {
        $mask = (1 << self::spareBits(strlen($seg))) - 1;
        $last = strpos(self::ALPHABET, $seg[strlen($seg) - 1]);
        $out = [];
        for ($low = 1; $low <= $mask; $low++) {
            $out[] = substr($seg, 0, -1) . self::ALPHABET[($last & ~$mask) | $low];
        }
        return $out;
    }

    /**
     * The issued compact JWS re-spelled: every other last character of the
     * signature, padded ('==' and, where two belong, '='), the standard
     * alphabet, LF / CR / CRLF inside the signature and, when
     * `$surroundingWhitespace`, a space before or after it and CRLF after it.
     *
     * @return array<string, string>
     */
    public static function of(string $issued, bool $surroundingWhitespace = true): array
    {
        [$h, $p, $sig] = explode('.', $issued);
        $head = "{$h}.{$p}.";
        $bytes = self::lenient($sig);
        $out = [];
        foreach (self::nonCanonicalSpellings($sig) as $v) {
            if (self::lenient($v) !== $bytes) {
                throw new \LogicException("test bug: {$v} is not another spelling of the signature");
            }
            $out['signature ends in ' . $v[strlen($v) - 1]] = $head . $v;
        }
        $pad = str_repeat('=', (4 - strlen($sig) % 4) % 4);
        if ($pad !== '') {
            $out['signature padded'] = $head . $sig . $pad;
        }
        if (strlen($pad) === 2) {
            $out['signature padded once'] = $head . $sig . '=';
        }
        $std = strtr($sig, '-_', '+/');
        if ($std !== $sig) {
            $out['signature in the standard alphabet'] = $head . $std;
        }
        $mid = intdiv(strlen($sig), 2);
        $out['line feed inside the signature'] = $head . substr($sig, 0, $mid) . "\n" . substr($sig, $mid);
        $out['carriage return inside the signature'] = $head . substr($sig, 0, $mid) . "\r" . substr($sig, $mid);
        $out['CRLF inside the signature'] = $head . substr($sig, 0, $mid) . "\r\n" . substr($sig, $mid);
        if ($surroundingWhitespace) {
            $out['leading space'] = ' ' . $issued;
            $out['trailing space'] = $issued . ' ';
            $out['CRLF after it'] = $issued . "\r\n";
        }
        return $out;
    }

    /**
     * The issued compact JWS changed in content: a signature bit flipped at
     * either end, a claim replaced, the payload's first character changed,
     * the signature one byte short, the value one character short, the
     * signature or its segment removed.
     *
     * @return array<string, string>
     */
    public static function tampered(string $issued, string $claim, string $otherClaim): array
    {
        [$h, $p, $s] = explode('.', $issued);
        $sig = self::lenient($s);
        $payload = self::lenient($p);
        if (!str_contains($payload, $claim) || $claim === $otherClaim) {
            throw new \LogicException("test bug: {$claim} must occur in the payload and differ from {$otherClaim}");
        }
        $first = $sig;
        $first[0] = chr(ord($first[0]) ^ 0x80);
        $last = $sig;
        $last[strlen($last) - 1] = chr(ord($last[strlen($last) - 1]) ^ 0x01);
        $swapped = $p[0] === 'A' ? 'B' : 'A';
        return [
            'first signature bit flipped' => "{$h}.{$p}." . self::enc($first),
            'last signature bit flipped' => "{$h}.{$p}." . self::enc($last),
            'payload claim changed' => "{$h}." . self::enc(str_replace($claim, $otherClaim, $payload)) . ".{$s}",
            'payload character changed' => "{$h}.{$swapped}" . substr($p, 1) . ".{$s}",
            'signature truncated by one byte' => "{$h}.{$p}." . self::enc(substr($sig, 0, -1)),
            'truncated by one character' => substr($issued, 0, -1),
            'signature removed' => "{$h}.{$p}.",
            'signature segment removed' => "{$h}.{$p}",
        ];
    }

    public static function enc(string $raw): string
    {
        return rtrim(strtr(base64_encode($raw), '+/', '-_'), '=');
    }

    private static function lenient(string $s): string
    {
        return (string) base64_decode(strtr(rtrim($s, '='), '-_', '+/'), false);
    }
}
