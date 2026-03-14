<?php

declare(strict_types=1);

namespace FlowCatalyst\Webhook;

use FlowCatalyst\Exceptions\WebhookValidationException;
use Illuminate\Http\Request;

/**
 * Validates incoming webhook signatures from FlowCatalyst using HMAC-SHA256.
 */
class WebhookValidator
{
    public function __construct(
        private readonly string $signingSecret
    ) {}

    /**
     * Validate a webhook signature.
     *
     * @param string $payload Raw request body
     * @param string $signature Value of X-FlowCatalyst-Signature header
     * @param string $timestamp Value of X-FlowCatalyst-Timestamp header
     * @param int $tolerance Max age in seconds (default 300 = 5 minutes)
     * @throws WebhookValidationException
     */
    public function validate(
        string $payload,
        string $signature,
        string $timestamp,
        int $tolerance = 300
    ): bool {
        // Validate timestamp
        $this->validateTimestamp($timestamp, $tolerance);

        // Compute expected signature
        $message = $timestamp . $payload;
        $expectedSignature = hash_hmac('sha256', $message, $this->signingSecret);

        // Constant-time comparison to prevent timing attacks
        if (!hash_equals($expectedSignature, $signature)) {
            throw WebhookValidationException::invalidSignature();
        }

        return true;
    }

    /**
     * Validate a webhook from a Laravel Request.
     *
     * @param Request $request The incoming request
     * @param int $tolerance Max age in seconds (default 300 = 5 minutes)
     * @throws WebhookValidationException
     */
    public function validateRequest(Request $request, int $tolerance = 300): bool
    {
        $signature = $request->header('X-FlowCatalyst-Signature');
        $timestamp = $request->header('X-FlowCatalyst-Timestamp');

        if (empty($signature)) {
            throw WebhookValidationException::missingSignature();
        }

        if (empty($timestamp)) {
            throw WebhookValidationException::missingTimestamp();
        }

        return $this->validate(
            payload: $request->getContent(),
            signature: $signature,
            timestamp: $timestamp,
            tolerance: $tolerance
        );
    }

    /**
     * Validate the timestamp is within tolerance.
     *
     * @throws WebhookValidationException
     */
    private function validateTimestamp(string $timestamp, int $tolerance): void
    {
        $webhookTime = (int) $timestamp;
        $currentTime = time();

        // Check if timestamp is too old
        if ($webhookTime < ($currentTime - $tolerance)) {
            throw WebhookValidationException::timestampExpired($tolerance);
        }

        // Check if timestamp is in the future (with 60 second grace period)
        if ($webhookTime > ($currentTime + 60)) {
            throw WebhookValidationException::timestampInFuture();
        }
    }

    /**
     * Create a validator from configuration.
     *
     * @throws WebhookValidationException
     */
    public static function fromConfig(): self
    {
        $secret = config('flowcatalyst.signing_secret');

        if (empty($secret)) {
            throw WebhookValidationException::missingSigningSecret();
        }

        return new self($secret);
    }
}
