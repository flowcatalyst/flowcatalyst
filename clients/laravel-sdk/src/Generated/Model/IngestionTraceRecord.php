<?php

namespace FlowCatalyst\Generated\Model;

class IngestionTraceRecord extends \ArrayObject
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
     * Unix milliseconds, the runtime's clock
     *
     * @var int|null
     */
    protected $at;
    /**
     * Shared by the records of one capture
     *
     * @var string|null
     */
    protected $captureId;
    /**
     * Cut to a stub over 80 KiB
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
    protected $ingestionId;
    /**
     * poll, exchange, http or error
     *
     * @var string|null
     */
    protected $kind;
    /**
     * info, warn or error
     *
     * @var string|null
     */
    protected $level;
    /**
     * One line; cut to 1000 characters
     *
     * @var string|null
     */
    protected $message;
    /**
     * @var int|null
     */
    protected $version;
    /**
     * @var string|null
     */
    protected $windowId;
    /**
     * Unix milliseconds, the runtime's clock
     *
     * @return int|null
     */
    public function getAt(): ?int
    {
        return $this->at;
    }
    /**
     * Unix milliseconds, the runtime's clock
     *
     * @param int|null $at
     *
     * @return self
     */
    public function setAt(?int $at): self
    {
        $this->initialized['at'] = true;
        $this->at = $at;
        return $this;
    }
    /**
     * Shared by the records of one capture
     *
     * @return string|null
     */
    public function getCaptureId(): ?string
    {
        return $this->captureId;
    }
    /**
     * Shared by the records of one capture
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
     * Cut to a stub over 80 KiB
     *
     * @return mixed
     */
    public function getData()
    {
        return $this->data;
    }
    /**
     * Cut to a stub over 80 KiB
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
    public function getIngestionId(): ?string
    {
        return $this->ingestionId;
    }
    /**
     * @param string|null $ingestionId
     *
     * @return self
     */
    public function setIngestionId(?string $ingestionId): self
    {
        $this->initialized['ingestionId'] = true;
        $this->ingestionId = $ingestionId;
        return $this;
    }
    /**
     * poll, exchange, http or error
     *
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * poll, exchange, http or error
     *
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
     * info, warn or error
     *
     * @return string|null
     */
    public function getLevel(): ?string
    {
        return $this->level;
    }
    /**
     * info, warn or error
     *
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
     * One line; cut to 1000 characters
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * One line; cut to 1000 characters
     *
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