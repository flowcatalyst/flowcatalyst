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
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $connection;
    /**
     * @var string|null
     */
    protected $mode;
    /**
     * @var string|null
     */
    protected $pool;
    /**
     * @var string|null
     */
    protected $serviceAccount;
    /**
     * @var string|null
     */
    protected $targetUrl;
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
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
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
     * @return string|null
     */
    public function getMode(): ?string
    {
        return $this->mode;
    }
    /**
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
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
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
     * @return string|null
     */
    public function getTargetUrl(): ?string
    {
        return $this->targetUrl;
    }
    /**
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
}