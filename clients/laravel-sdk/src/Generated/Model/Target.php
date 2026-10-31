<?php

namespace FlowCatalyst\Generated\Model;

class Target
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
     * @var int|null
     */
    protected $batchSize;
    /**
     * A dispatch job's code, application:subdomain:aggregate:action; for an EVENT target, the code of an existing event type
     *
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $connection;
    /**
     * DISPATCH_JOB only: deliver the record as it is, with its own content type, instead of the platform envelope
     *
     * @var bool|null
     */
    protected $dataOnly;
    /**
     * What each record becomes; DISPATCH_JOB when absent
     *
     * @var string|null
     */
    protected $kind;
    /**
     * Not for an EVENT target
     *
     * @var string|null
     */
    protected $mode;
    /**
     * Not for an EVENT target
     *
     * @var string|null
     */
    protected $pool;
    /**
     * @var string|null
     */
    protected $serviceAccount;
    /**
     * The delivery URL. May be left out only when the source names a config entry whose value gives it (exactly one of the two). Not for an EVENT target
     *
     * @var string|null
     */
    protected $targetUrl;
    /**
     * EVENT only: deliver a record that is not JSON as {contentType, body}. Required unless the records are known to be JSON
     *
     * @var bool|null
     */
    protected $wrapNonJson;
    /**
     * @return int|null
     */
    public function getBatchSize(): ?int
    {
        return $this->batchSize;
    }
    /**
     * @param int|null $batchSize
     *
     * @return self
     */
    public function setBatchSize(?int $batchSize): self
    {
        $this->initialized['batchSize'] = true;
        $this->batchSize = $batchSize;
        return $this;
    }
    /**
     * A dispatch job's code, application:subdomain:aggregate:action; for an EVENT target, the code of an existing event type
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * A dispatch job's code, application:subdomain:aggregate:action; for an EVENT target, the code of an existing event type
     *
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getConnection(): ?string
    {
        return $this->connection;
    }
    /**
     * @param string|null $connection
     *
     * @return self
     */
    public function setConnection(?string $connection): self
    {
        $this->initialized['connection'] = true;
        $this->connection = $connection;
        return $this;
    }
    /**
     * DISPATCH_JOB only: deliver the record as it is, with its own content type, instead of the platform envelope
     *
     * @return bool|null
     */
    public function getDataOnly(): ?bool
    {
        return $this->dataOnly;
    }
    /**
     * DISPATCH_JOB only: deliver the record as it is, with its own content type, instead of the platform envelope
     *
     * @param bool|null $dataOnly
     *
     * @return self
     */
    public function setDataOnly(?bool $dataOnly): self
    {
        $this->initialized['dataOnly'] = true;
        $this->dataOnly = $dataOnly;
        return $this;
    }
    /**
     * What each record becomes; DISPATCH_JOB when absent
     *
     * @return string|null
     */
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * What each record becomes; DISPATCH_JOB when absent
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
     * Not for an EVENT target
     *
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
     * Not for an EVENT target
     *
     * @param string|null $mode
     *
     * @return self
     */
    public function setMode(?string $mode): self
    {
        $this->initialized['mode'] = true;
        $this->mode = $mode;
        return $this;
    }
    /**
     * Not for an EVENT target
     *
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
     * Not for an EVENT target
     *
     * @param string|null $pool
     *
     * @return self
     */
    public function setPool(?string $pool): self
    {
        $this->initialized['pool'] = true;
        $this->pool = $pool;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getServiceAccount(): ?string
    {
        return $this->serviceAccount;
    }
    /**
     * @param string|null $serviceAccount
     *
     * @return self
     */
    public function setServiceAccount(?string $serviceAccount): self
    {
        $this->initialized['serviceAccount'] = true;
        $this->serviceAccount = $serviceAccount;
        return $this;
    }
    /**
     * The delivery URL. May be left out only when the source names a config entry whose value gives it (exactly one of the two). Not for an EVENT target
     *
     * @return string|null
     */
    public function getTargetUrl(): ?string
    {
        return $this->targetUrl;
    }
    /**
     * The delivery URL. May be left out only when the source names a config entry whose value gives it (exactly one of the two). Not for an EVENT target
     *
     * @param string|null $targetUrl
     *
     * @return self
     */
    public function setTargetUrl(?string $targetUrl): self
    {
        $this->initialized['targetUrl'] = true;
        $this->targetUrl = $targetUrl;
        return $this;
    }
    /**
     * EVENT only: deliver a record that is not JSON as {contentType, body}. Required unless the records are known to be JSON
     *
     * @return bool|null
     */
    public function getWrapNonJson(): ?bool
    {
        return $this->wrapNonJson;
    }
    /**
     * EVENT only: deliver a record that is not JSON as {contentType, body}. Required unless the records are known to be JSON
     *
     * @param bool|null $wrapNonJson
     *
     * @return self
     */
    public function setWrapNonJson(?bool $wrapNonJson): self
    {
        $this->initialized['wrapNonJson'] = true;
        $this->wrapNonJson = $wrapNonJson;
        return $this;
    }
}