<?php

namespace FlowCatalyst\Generated\Model;

class SetChange
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
     * @var string|null
     */
    protected $action;
    /**
     * @var string|null
     */
    protected $application;
    /**
     * @var list<string>|null
     */
    protected $changed;
    /**
     * @var string|null
     */
    protected $client;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $ingestionId;
    /**
     * @return string|null
     */
    public function getAction(): ?string
    {
        return $this->action;
    }
    /**
     * @param string|null $action
     *
     * @return self
     */
    public function setAction(?string $action): self
    {
        $this->initialized['action'] = true;
        $this->action = $action;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplication(): ?string
    {
        return $this->application;
    }
    /**
     * @param string|null $application
     *
     * @return self
     */
    public function setApplication(?string $application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getChanged(): ?array
    {
        return $this->changed;
    }
    /**
     * @param list<string>|null $changed
     *
     * @return self
     */
    public function setChanged(?array $changed): self
    {
        $this->initialized['changed'] = true;
        $this->changed = $changed;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClient(): ?string
    {
        return $this->client;
    }
    /**
     * @param string|null $client
     *
     * @return self
     */
    public function setClient(?string $client): self
    {
        $this->initialized['client'] = true;
        $this->client = $client;
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
}