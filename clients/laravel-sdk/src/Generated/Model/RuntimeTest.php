<?php

namespace FlowCatalyst\Generated\Model;

class RuntimeTest
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
    protected $expiresAt;
    /**
     * Answer it in the next report under tests
     *
     * @var string|null
     */
    protected $id;
    /**
     * @var RuntimeIngestion|null
     */
    protected $ingestion;
    /**
     * @return \DateTime|null
     */
    public function getExpiresAt(): ?\DateTime
    {
        return $this->expiresAt;
    }
    /**
     * @param \DateTime|null $expiresAt
     *
     * @return self
     */
    public function setExpiresAt(?\DateTime $expiresAt): self
    {
        $this->initialized['expiresAt'] = true;
        $this->expiresAt = $expiresAt;
        return $this;
    }
    /**
     * Answer it in the next report under tests
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Answer it in the next report under tests
     *
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
     * @return RuntimeIngestion|null
     */
    public function getIngestion(): ?RuntimeIngestion
    {
        return $this->ingestion;
    }
    /**
     * @param RuntimeIngestion|null $ingestion
     *
     * @return self
     */
    public function setIngestion(?RuntimeIngestion $ingestion): self
    {
        $this->initialized['ingestion'] = true;
        $this->ingestion = $ingestion;
        return $this;
    }
}