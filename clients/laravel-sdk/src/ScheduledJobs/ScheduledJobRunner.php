<?php

declare(strict_types=1);

namespace FlowCatalyst\ScheduledJobs;

use FlowCatalyst\Client\Resources\ScheduledJobs;
use Throwable;

/**
 * ScheduledJobRunner — handler registry + envelope dispatch + lock + optional
 * completion callback. Mount `processWithResponse()` from a Laravel controller
 * route at the URL you set as `targetUrl` on the job definition.
 *
 *   Route::post('/_fc/scheduled-jobs/process', function (Request $req) use ($runner) {
 *       [$status, $body] = $runner->processWithResponse($req->json()->all());
 *       return response()->json($body, $status);
 *   });
 *
 * Returns 202 immediately on a recognised envelope and runs the handler
 * synchronously before returning — Laravel doesn't have native fire-and-
 * forget HTTP semantics, so handlers should keep work brief or queue it
 * onto Laravel's job queue themselves. The platform's HTTP timeout
 * (default 10s) is the deadline.
 */
final class ScheduledJobRunner
{
    /** @var array<string, callable> */
    private array $handlers = [];

    private LockProvider $lockProvider;
    /** @var (callable(array): string)|null */
    private $lockKeyResolver;
    private bool $enforceLock;
    private int $lockTtlSeconds;
    /** @var (callable(Throwable, array): void)|null */
    private $onError;

    /**
     * @param array{
     *   lockProvider?: LockProvider,
     *   lockKey?: callable(array): string,
     *   enforceLock?: bool,
     *   lockTtlSeconds?: int,
     *   onError?: callable(Throwable, array): void,
     * } $options
     */
    public function __construct(
        private readonly ScheduledJobs $resource,
        array $options = [],
    ) {
        $this->lockProvider = $options['lockProvider'] ?? new NoOpLockProvider();
        $this->lockKeyResolver = $options['lockKey'] ?? null;
        $this->enforceLock = $options['enforceLock'] ?? true;
        $this->lockTtlSeconds = $options['lockTtlSeconds'] ?? 600;
        $this->onError = $options['onError'] ?? null;
    }

    /**
     * Register a handler keyed by the job's `code`. The handler receives the
     * envelope as an array and a logger callable, and may return any
     * JSON-serialisable value (used as `completion_result` when the job
     * tracks completion).
     *
     * @param callable(array, callable(string, string=, mixed=): void): mixed $handler
     */
    public function handler(string $code, callable $handler): self
    {
        $this->handlers[$code] = $handler;
        return $this;
    }

    /** @return string[] */
    public function registeredCodes(): array
    {
        return array_keys($this->handlers);
    }

    /**
     * Process an inbound platform → SDK firing.
     *
     * @param array<string, mixed> $envelope
     * @return array{0: int, 1: array<string, mixed>} [status, bodyJson]
     */
    public function processWithResponse(array $envelope): array
    {
        $error = $this->validateEnvelope($envelope);
        if ($error !== null) {
            return [400, ['error' => $error]];
        }

        $code = (string) $envelope['jobCode'];
        if (!isset($this->handlers[$code])) {
            return [404, ['error' => "No handler registered for code '{$code}'"]];
        }

        $this->runOne($envelope, $this->handlers[$code]);
        return [202, ['ok' => true]];
    }

    /**
     * @param array<string, mixed> $envelope
     */
    private function runOne(array $envelope, callable $handler): void
    {
        $release = null;
        try {
            if ($this->enforceLock) {
                $key = $this->lockKeyResolver !== null
                    ? ($this->lockKeyResolver)($envelope)
                    : 'scheduled-job:' . $envelope['jobCode'];
                $release = $this->lockProvider->acquire($key, $this->lockTtlSeconds);
                if ($release === null) {
                    if ((bool) ($envelope['tracksCompletion'] ?? false)) {
                        $this->safeComplete($envelope, 'FAILURE', [
                            'skipped' => true,
                            'reason' => 'lock-held',
                        ]);
                    }
                    return;
                }
            }

            $instanceId = (string) $envelope['instanceId'];
            $logger = function (string $message, string $level = 'INFO', mixed $metadata = null) use ($instanceId): void {
                try {
                    $this->resource->logForInstance($instanceId, $message, $level, $metadata);
                } catch (Throwable $e) {
                    if ($this->onError) ($this->onError)($e, ['instanceId' => $instanceId]);
                }
            };

            $succeeded = true;
            $result = null;
            $thrown = null;
            try {
                $result = $handler($envelope, $logger);
            } catch (Throwable $e) {
                $succeeded = false;
                $thrown = $e;
            }

            if ((bool) ($envelope['tracksCompletion'] ?? false)) {
                $this->safeComplete(
                    $envelope,
                    $succeeded ? 'SUCCESS' : 'FAILURE',
                    $succeeded
                        ? $this->sanitiseResult($result)
                        : ['error' => $thrown?->getMessage() ?? 'Unknown error'],
                );
            }

            if (!$succeeded && $this->onError) {
                ($this->onError)($thrown, $envelope);
            }
        } finally {
            if ($release !== null) {
                try { $release(); } catch (Throwable $e) {
                    if ($this->onError) ($this->onError)($e, $envelope);
                }
            }
        }
    }

    /**
     * @param array<string, mixed> $envelope
     */
    private function safeComplete(array $envelope, string $status, mixed $result): void
    {
        try {
            $this->resource->completeInstance(
                (string) $envelope['instanceId'],
                $status,
                $result,
            );
        } catch (Throwable $e) {
            if ($this->onError) ($this->onError)($e, $envelope);
        }
    }

    /**
     * @param array<string, mixed> $envelope
     */
    private function validateEnvelope(array $envelope): ?string
    {
        foreach (['jobId', 'jobCode', 'instanceId', 'firedAt', 'triggerKind'] as $required) {
            if (!isset($envelope[$required]) || !is_string($envelope[$required])) {
                return "Envelope missing string field '{$required}'";
            }
        }
        if (!isset($envelope['tracksCompletion']) || !is_bool($envelope['tracksCompletion'])) {
            return "Envelope missing boolean field 'tracksCompletion'";
        }
        $trigger = (string) $envelope['triggerKind'];
        if ($trigger !== 'CRON' && $trigger !== 'MANUAL') {
            return "Invalid triggerKind '{$trigger}'";
        }
        return null;
    }

    private function sanitiseResult(mixed $v): mixed
    {
        // Cap at ~10KB so completion_result doesn't bloat the row.
        try {
            $json = json_encode($v, JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE);
            if ($json === false) return ['unserialisable' => true];
            if (strlen($json) > 10_000) {
                return ['truncated' => true, 'preview' => substr($json, 0, 10_000)];
            }
            return $v;
        } catch (Throwable) {
            return ['unserialisable' => true];
        }
    }
}
