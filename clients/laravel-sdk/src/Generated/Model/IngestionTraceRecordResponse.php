<?php

namespace FlowCatalyst\Generated\Model;

class IngestionTraceRecordResponse
{
    /**
     * @var array
     */
    protected $initialized = [];
    public function isInitialized($property): bool
    {
        return array_key_exists($property, $this->initialized);
    }
    /**
     * @var \DateTime|null
     */
    protected $at;
    /**
     * Shared by the records of one capture: a poll cycle or one exchange
     *
     * @var string|null
     */
    protected $captureId;
    /**
     * The record's data, by kind
     *
     * @var mixed|null
     */
    protected $data;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $level;
    /**
     * @var string|null
     */
    protected $message;
    /**
     * Sensitive parts withheld (no platform:ingestion:trace:view-bodies): an http record keeps the client, method, URL path, status, sizes, duration, attempt and failure; an exchange record its counts and outcome; a poll record all but its error; an error record nothing in data. The message of an http, exchange or error record is then rebuilt from what stays.
     *
     * @var bool|null
     */
    protected $redacted;
    /**
     * @var string|null
     */
    protected $runtimeId;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * @var string|null
     */
    protected $windowId;
    /**
     * @return \DateTime|null
     */
    public function getAt(): ?\DateTime
    {
        return $this->at;
    }
    /**
     * @param \DateTime|null $at
     *
     * @return self
     */
    public function setAt(?\DateTime $at): self
    {
        $this->initialized['at'] = true;
        $this->at = $at;
        return $this;
    }
    /**
     * Shared by the records of one capture: a poll cycle or one exchange
     *
     * @return string|null
     */
    public function getCaptureId(): ?string
    {
        return $this->captureId;
    }
    /**
     * Shared by the records of one capture: a poll cycle or one exchange
     *
     * @param string|null $captureId
     *
     * @return self
     */
    public function setCaptureId(?string $captureId): self
    {
        $this->initialized['captureId'] = true;
        $this->captureId = $captureId;
        return $this;
    }
    /**
     * The record's data, by kind
     *
     * @return mixed
     */
    public function getData()
    {
        return $this->data;
    }
    /**
     * The record's data, by kind
     *
     * @param mixed $data
     *
     * @return self
     */
    public function setData($data): self
    {
        $this->initialized['data'] = true;
        $this->data = $data;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLevel(): ?string
    {
        return $this->level;
    }
    /**
     * @param string|null $level
     *
     * @return self
     */
    public function setLevel(?string $level): self
    {
        $this->initialized['level'] = true;
        $this->level = $level;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * Sensitive parts withheld (no platform:ingestion:trace:view-bodies): an http record keeps the client, method, URL path, status, sizes, duration, attempt and failure; an exchange record its counts and outcome; a poll record all but its error; an error record nothing in data. The message of an http, exchange or error record is then rebuilt from what stays.
     *
     * @return bool|null
     */
    public function getRedacted(): ?bool
    {
        return $this->redacted;
    }
    /**
     * Sensitive parts withheld (no platform:ingestion:trace:view-bodies): an http record keeps the client, method, URL path, status, sizes, duration, attempt and failure; an exchange record its counts and outcome; a poll record all but its error; an error record nothing in data. The message of an http, exchange or error record is then rebuilt from what stays.
     *
     * @param bool|null $redacted
     *
     * @return self
     */
    public function setRedacted(?bool $redacted): self
    {
        $this->initialized['redacted'] = true;
        $this->redacted = $redacted;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getVersion(): ?int
    {
        return $this->version;
    }
    /**
     * @param int|null $version
     *
     * @return self
     */
    public function setVersion(?int $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWindowId(): ?string
    {
        return $this->windowId;
    }
    /**
     * @param string|null $windowId
     *
     * @return self
     */
    public function setWindowId(?string $windowId): self
    {
        $this->initialized['windowId'] = true;
        $this->windowId = $windowId;
        return $this;
    }
}