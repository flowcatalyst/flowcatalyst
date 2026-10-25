<?php

namespace FlowCatalyst\Generated\Model;

class JobResult
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
    protected $error;
    /**
     * @var bool|null
     */
    protected $existing;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var int|null
     */
    protected $index;
    /**
     * @return string|null
     */
    public function getError(): ?string
    {
        return $this->error;
    }
    /**
     * @param string|null $error
     *
     * @return self
     */
    public function setError(?string $error): self
    {
        $this->initialized['error'] = true;
        $this->error = $error;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getExisting(): ?bool
    {
        return $this->existing;
    }
    /**
     * @param bool|null $existing
     *
     * @return self
     */
    public function setExisting(?bool $existing): self
    {
        $this->initialized['existing'] = true;
        $this->existing = $existing;
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
     * @return int|null
     */
    public function getIndex(): ?int
    {
        return $this->index;
    }
    /**
     * @param int|null $index
     *
     * @return self
     */
    public function setIndex(?int $index): self
    {
        $this->initialized['index'] = true;
        $this->index = $index;
        return $this;
    }
}