<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSecretResponse
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
     * The live definition names it, so its runtime may fetch it; false for a name only fetched under an earlier live version
     *
     * @var bool|null
     */
    protected $bound;
    /**
     * Per runtime, newest first; empty when no runtime has fetched it
     *
     * @var list<IngestionSecretFetchResponse>|null
     */
    protected $fetches;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * The live definition names it, so its runtime may fetch it; false for a name only fetched under an earlier live version
     *
     * @return bool|null
     */
    public function getBound(): ?bool
    {
        return $this->bound;
    }
    /**
     * The live definition names it, so its runtime may fetch it; false for a name only fetched under an earlier live version
     *
     * @param bool|null $bound
     *
     * @return self
     */
    public function setBound(?bool $bound): self
    {
        $this->initialized['bound'] = true;
        $this->bound = $bound;
        return $this;
    }
    /**
     * Per runtime, newest first; empty when no runtime has fetched it
     *
     * @return list<IngestionSecretFetchResponse>|null
     */
    public function getFetches(): ?array
    {
        return $this->fetches;
    }
    /**
     * Per runtime, newest first; empty when no runtime has fetched it
     *
     * @param list<IngestionSecretFetchResponse>|null $fetches
     *
     * @return self
     */
    public function setFetches(?array $fetches): self
    {
        $this->initialized['fetches'] = true;
        $this->fetches = $fetches;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
}