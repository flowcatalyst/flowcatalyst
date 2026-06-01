<?php

declare(strict_types=1);

namespace FlowCatalyst\Attributes;

use Attribute;

/**
 * Marks a class as a scheduled-job definition for FlowCatalyst.
 *
 * Usage:
 * ```php
 * #[AsScheduledJob(
 *     code: 'nightly-report',
 *     name: 'Nightly Report',
 *     crons: ['0 2 * * *'],
 *     timezone: 'Europe/London',
 *     description: 'Aggregate prior-day metrics and email to ops',
 *     concurrent: false,
 *     tracksCompletion: true,
 *     timeoutSeconds: 600,
 * )]
 * class NightlyReportJob {}
 * ```
 *
 * The application code is added from your config when synced — e.g. with
 * app code "orders" the platform stores this job as "orders:nightly-report".
 *
 * `crons` accepts standard 5-field expressions; `timezone` defaults to UTC
 * server-side when omitted. `concurrent` controls whether the platform
 * fires a new tick while a previous invocation is still running (default
 * false). `tracksCompletion` flips the platform from "webhook delivery is
 * the success signal" to "consumer POSTs to /complete when done".
 */
#[Attribute(Attribute::TARGET_CLASS)]
final class AsScheduledJob
{
    /**
     * @param string $code Short code (no `<app>:` prefix — added at sync time)
     * @param string $name Human-friendly label
     * @param string[] $crons Standard 5-field cron expressions
     * @param string|null $description Short summary
     * @param string|null $timezone IANA tz name (default UTC server-side)
     * @param array<string, mixed>|null $payload JSON payload sent to the consumer
     * @param bool $concurrent Allow overlapping invocations (default false)
     * @param bool $tracksCompletion Consumer reports via /complete (default false)
     * @param int|null $timeoutSeconds Per-invocation timeout
     * @param int|null $deliveryMaxAttempts Webhook delivery retries (default 3)
     * @param string|null $targetUrl Override the application's default callback URL
     */
    public function __construct(
        public readonly string $code,
        public readonly string $name,
        public readonly array $crons,
        public readonly ?string $description = null,
        public readonly ?string $timezone = null,
        public readonly ?array $payload = null,
        public readonly bool $concurrent = false,
        public readonly bool $tracksCompletion = false,
        public readonly ?int $timeoutSeconds = null,
        public readonly ?int $deliveryMaxAttempts = null,
        public readonly ?string $targetUrl = null,
    ) {}

    /**
     * @return array<string, mixed>
     */
    public function toArray(): array
    {
        $data = [
            'code' => $this->code,
            'name' => $this->name,
            'crons' => $this->crons,
            'concurrent' => $this->concurrent,
            'tracksCompletion' => $this->tracksCompletion,
        ];
        if ($this->description !== null) {
            $data['description'] = $this->description;
        }
        if ($this->timezone !== null) {
            $data['timezone'] = $this->timezone;
        }
        if ($this->payload !== null) {
            $data['payload'] = $this->payload;
        }
        if ($this->timeoutSeconds !== null) {
            $data['timeoutSeconds'] = $this->timeoutSeconds;
        }
        if ($this->deliveryMaxAttempts !== null) {
            $data['deliveryMaxAttempts'] = $this->deliveryMaxAttempts;
        }
        if ($this->targetUrl !== null) {
            $data['targetUrl'] = $this->targetUrl;
        }
        return $data;
    }
}
