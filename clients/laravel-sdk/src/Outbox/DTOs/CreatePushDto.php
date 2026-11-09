<?php

declare(strict_types=1);

namespace FlowCatalyst\Outbox\DTOs;

use FlowCatalyst\Exceptions\PushException;

/**
 * A push to the clients of an application, written to the outbox in the same
 * transaction as the change it announces (`Outbox::createPush`, or
 * `Plan::andPush` in a use case). The platform's outbox processor sends the row
 * to the push gateway (`POST /api/push/batch`), which fans it out to the
 * clients listening on the topic over server-sent events. Pushes are signals:
 * the clients read the change again; durable work is events and dispatch jobs.
 *
 * The rules are the push contract's section 1, the refusal codes those of
 * `conformance/push/topics.json` ({@see PushException}), and the row / payload
 * shape that of the Go SDK (`pkg/fcsdk/outbox/push.go`); the shared vector
 * `outbox-push.json` pins all three.
 *
 * Immutable: the `with*` methods return a copy.
 */
final class CreatePushDto
{
    /** Longest topic, in characters. */
    public const MAX_TOPIC_LENGTH = 255;
    /** Longest type, in characters. */
    public const MAX_TYPE_LENGTH = 100;
    /** Default data limit in bytes (`push.max-data-bytes`). */
    public const DEFAULT_MAX_DATA_BYTES = 16384;
    /** The most the data limit may be set to. */
    public const MAX_DATA_BYTES = 65536;
    /** The client code that means "the platform": left out of the payload. */
    public const PLATFORM_CLIENT_CODE = '_platform';
    /** The Postgres notification channel a PUSH row's writer NOTIFYs in the same statement as the insert. */
    public const NOTIFY_CHANNEL = 'fc_outbox_push';

    private const JSON_FLAGS = JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES;

    /**
     * @param string $topic `{application}:{segment}(:{segment})*`; the first segment is the application
     *        (your credential must be allowed to push for it), the others `[0-9A-Za-z_.-]+`, at most 255 characters.
     * @param string $type `[a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*`, at most 100 characters; `reset`,
     *        `auth-expired`, `subscribed` and `fw.*` are reserved.
     * @param array<string, mixed>|object|string $data What the listeners receive: a JSON object — an associative
     *        array (an empty array is `{}`), an object, or JSON text (a string) holding one. Anything else
     *        (a list, a scalar, null) is refused with DATA_NOT_OBJECT.
     * @param string|null $id The row id and the payload's `pushId` (the platform writes a pushId once);
     *        a TSID is generated when null. At most 26 characters `[0-9A-Za-z_-]`.
     * @param string|null $clientCode The client the push is for, by identifier. Null: the writer's
     *        configured client code (`flowcatalyst.outbox.client_code`; `_platform` leaves it out).
     * @param string|null $messageGroup The outbox message group. Null: the topic, so the pushes of one
     *        topic reach the gateway in order. Ignored when $ungrouped.
     * @param bool $ungrouped No message group: batched, concurrent, unordered. For signal-only topics.
     * @param int|null $maxDataBytes The data limit (default 16 KiB, at most 64 KiB): the application's
     *        `push.max-data-bytes`.
     */
    public function __construct(
        public readonly string $topic,
        public readonly string $type,
        public readonly mixed $data,
        public readonly ?string $id = null,
        public readonly ?string $clientCode = null,
        public readonly ?string $messageGroup = null,
        public readonly bool $ungrouped = false,
        public readonly ?int $maxDataBytes = null,
    ) {}

    /**
     * @param array<string, mixed>|object|string $data
     */
    public static function create(string $topic, string $type, array|object|string $data): self
    {
        return new self($topic, $type, $data);
    }

    /**
     * @param array<string, mixed> $changes constructor argument name => value
     */
    private function with(array $changes): self
    {
        return new self(...array_merge(get_object_vars($this), $changes));
    }

    public function withId(string $id): self
    {
        return $this->with(['id' => $id]);
    }

    public function withClientCode(string $clientCode): self
    {
        return $this->with(['clientCode' => $clientCode]);
    }

    /**
     * Replace the topic as the message group. Ignored when {@see withUngrouped()}.
     */
    public function withMessageGroup(string $messageGroup): self
    {
        return $this->with(['messageGroup' => $messageGroup]);
    }

    /**
     * Write the row with no message group: the processor batches such rows into
     * concurrent calls (faster, no order). For signal-only topics.
     */
    public function withUngrouped(): self
    {
        return $this->with(['ungrouped' => true]);
    }

    public function withMaxDataBytes(int $maxDataBytes): self
    {
        return $this->with(['maxDataBytes' => $maxDataBytes]);
    }

    private function limit(): int
    {
        return $this->maxDataBytes !== null && $this->maxDataBytes > 0
            ? min($this->maxDataBytes, self::MAX_DATA_BYTES)
            : self::DEFAULT_MAX_DATA_BYTES;
    }

    /**
     * Apply the push contract's section 1 (topic, type, data) and the row's
     * own rules (id, client code, group), in the Go SDK's order.
     *
     * @throws PushException
     */
    public function validate(): void
    {
        $this->compactData();
    }

    /**
     * Validate and serialise as the PUSH row's payload JSON text, members in the
     * framework's order: `topic`, `type`, `data`, `clientCode`, `pushId`. `data`
     * is the object itself, never encoded JSON in a string.
     *
     * @param string $id The row id (also the `pushId`).
     * @param string|null $defaultClientCode The writer's client code, used when the push names none;
     *        `_platform` leaves `clientCode` out.
     * @throws PushException
     */
    public function toPayloadJson(string $id, ?string $defaultClientCode = null): string
    {
        $data = $this->compactData();

        $code = trim($this->clientCode ?? '');
        if ($code === '') {
            $default = trim($defaultClientCode ?? '');
            if ($default !== self::PLATFORM_CLIENT_CODE) {
                $code = $default;
            }
        }

        return '{"topic":' . json_encode($this->topic, self::JSON_FLAGS)
            . ',"type":' . json_encode($this->type, self::JSON_FLAGS)
            . ',"data":' . $data
            . ($code !== '' ? ',"clientCode":' . json_encode($code, self::JSON_FLAGS) : '')
            . ',"pushId":' . json_encode($id, self::JSON_FLAGS) . '}';
    }

    /**
     * The message group the row is written with: none when ungrouped, else the
     * override, else the topic.
     */
    public function resolvedMessageGroup(): ?string
    {
        if ($this->ungrouped) {
            return null;
        }
        return $this->messageGroup !== null && $this->messageGroup !== '' ? $this->messageGroup : $this->topic;
    }

    /**
     * Run every rule and return the data as compact JSON text.
     *
     * @throws PushException
     */
    private function compactData(): string
    {
        $topic = $this->topic;
        if (strlen($topic) > self::MAX_TOPIC_LENGTH) {
            throw new PushException(PushException::CODE_TOPIC_INVALID, 'a topic is at most ' . self::MAX_TOPIC_LENGTH . ' characters');
        }
        $segments = explode(':', $topic);
        if (count($segments) < 2 || preg_match('/^[a-z][a-z0-9-]*$/D', $segments[0]) !== 1) {
            throw new PushException(PushException::CODE_TOPIC_INVALID, "a topic is {application}:{segment}(:{segment})*: \"{$topic}\"");
        }
        foreach (array_slice($segments, 1) as $segment) {
            if (preg_match('/^[0-9A-Za-z_.-]+$/D', $segment) !== 1) {
                throw new PushException(PushException::CODE_TOPIC_INVALID, "a topic segment is [0-9A-Za-z_.-]+: \"{$topic}\"");
            }
        }

        $type = $this->type;
        if (strlen($type) > self::MAX_TYPE_LENGTH) {
            throw new PushException(PushException::CODE_TYPE_INVALID, 'a type is at most ' . self::MAX_TYPE_LENGTH . ' characters');
        }
        foreach (explode('.', $type) as $part) {
            if (preg_match('/^[a-z][a-z0-9-]*$/D', $part) !== 1) {
                throw new PushException(PushException::CODE_TYPE_INVALID, "a type is [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*: \"{$type}\"");
            }
        }
        if ($type === 'reset' || $type === 'auth-expired' || $type === 'subscribed' || str_starts_with($type, 'fw.')) {
            throw new PushException(PushException::CODE_TYPE_RESERVED, "the type \"{$type}\" is reserved");
        }

        $text = $this->dataText();
        $size = strlen($text);
        if ($size > $this->limit()) {
            throw new PushException(PushException::CODE_DATA_TOO_LARGE, "the data is {$size} bytes, more than {$this->limit()}");
        }

        if ($this->id !== null && $this->id !== '' && preg_match('/^[0-9A-Za-z_-]{1,26}$/D', $this->id) !== 1) {
            throw new PushException(PushException::CODE_PUSH_ID_INVALID, 'id must match [0-9A-Za-z_-]{1,26}');
        }
        if ($this->clientCode !== null && $this->clientCode !== '' && (strlen($this->clientCode) > 100 || trim($this->clientCode) === '')) {
            throw new PushException(PushException::CODE_CLIENT_INVALID, 'clientCode must be 1 to 100 characters');
        }
        if ($this->messageGroup !== null && strlen($this->messageGroup) > 255) {
            throw new PushException(PushException::CODE_TOPIC_INVALID, 'messageGroup is at most 255 characters');
        }

        return $text;
    }

    /**
     * The data as compact JSON text, checked to be a JSON object.
     *
     * @throws PushException
     */
    private function dataText(): string
    {
        $data = $this->data;

        if (is_string($data)) {
            $text = trim($data);
            if ($text === '') {
                throw new PushException(PushException::CODE_DATA_INVALID, 'the data is not valid JSON');
            }
            json_decode($text);
            if (json_last_error() !== JSON_ERROR_NONE) {
                throw new PushException(PushException::CODE_DATA_INVALID, 'the data is not valid JSON');
            }
            if ($text[0] !== '{') {
                throw new PushException(PushException::CODE_DATA_NOT_OBJECT, 'the data is not a JSON object');
            }
            return self::compactJson($text);
        }

        if (is_array($data)) {
            // An associative array is an object; an empty array is `{}`; a list is not an object.
            if ($data === []) {
                return '{}';
            }
            if (array_is_list($data)) {
                throw new PushException(PushException::CODE_DATA_NOT_OBJECT, 'the data is not a JSON object');
            }
        } elseif (!is_object($data)) {
            throw new PushException(PushException::CODE_DATA_NOT_OBJECT, 'the data is not a JSON object');
        }

        try {
            $text = json_encode($data, self::JSON_FLAGS | JSON_THROW_ON_ERROR);
        } catch (\JsonException $e) {
            throw new PushException(PushException::CODE_DATA_INVALID, 'the data does not marshal to JSON: ' . $e->getMessage());
        }
        if ($text === '' || $text[0] !== '{') {
            throw new PushException(PushException::CODE_DATA_NOT_OBJECT, 'the data is not a JSON object');
        }
        return $text;
    }

    /**
     * Whitespace-strip JSON text without re-serialising its values.
     */
    private static function compactJson(string $text): string
    {
        $out = '';
        $inString = false;
        $escaped = false;
        $length = strlen($text);
        for ($i = 0; $i < $length; $i++) {
            $ch = $text[$i];
            if ($inString) {
                $out .= $ch;
                if ($escaped) {
                    $escaped = false;
                } elseif ($ch === '\\') {
                    $escaped = true;
                } elseif ($ch === '"') {
                    $inString = false;
                }
                continue;
            }
            if ($ch === '"') {
                $inString = true;
                $out .= $ch;
            } elseif ($ch !== ' ' && $ch !== "\t" && $ch !== "\n" && $ch !== "\r") {
                $out .= $ch;
            }
        }
        return $out;
    }
}
