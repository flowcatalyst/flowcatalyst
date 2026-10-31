<?php

namespace FlowCatalyst\Generated\Model;

class IngestionConfigDeletedResponse
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
     * A URL to the JSON Schema for this object.
     *
     * @var string|null
     */
    protected $dollarSchema;
    /**
     * The codes of every ingestion that names the entry; each is now MISSING and does not run until the entry is set again
     *
     * @var list<string>|null
     */
    protected $ingestions;
    /**
     * @var string|null
     */
    protected $key;
    /**
     * A URL to the JSON Schema for this object.
     *
     * @return string|null
     */
    public function getDollarSchema(): ?string
    {
        return $this->dollarSchema;
    }
    /**
     * A URL to the JSON Schema for this object.
     *
     * @param string|null $dollarSchema
     *
     * @return self
     */
    public function setDollarSchema(?string $dollarSchema): self
    {
        $this->initialized['dollarSchema'] = true;
        $this->dollarSchema = $dollarSchema;
        return $this;
    }
    /**
     * The codes of every ingestion that names the entry; each is now MISSING and does not run until the entry is set again
     *
     * @return list<string>|null
     */
    public function getIngestions(): ?array
    {
        return $this->ingestions;
    }
    /**
     * The codes of every ingestion that names the entry; each is now MISSING and does not run until the entry is set again
     *
     * @param list<string>|null $ingestions
     *
     * @return self
     */
    public function setIngestions(?array $ingestions): self
    {
        $this->initialized['ingestions'] = true;
        $this->ingestions = $ingestions;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getKey(): ?string
    {
        return $this->key;
    }
    /**
     * @param string|null $key
     *
     * @return self
     */
    public function setKey(?string $key): self
    {
        $this->initialized['key'] = true;
        $this->key = $key;
        return $this;
    }
}