<?php

namespace FlowCatalyst\Generated\Model;

class MissingFilesResponse
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
     * The SHA-256 of every file the application has not stored
     *
     * @var list<string>|null
     */
    protected $missing;
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
     * The SHA-256 of every file the application has not stored
     *
     * @return list<string>|null
     */
    public function getMissing(): ?array
    {
        return $this->missing;
    }
    /**
     * The SHA-256 of every file the application has not stored
     *
     * @param list<string>|null $missing
     *
     * @return self
     */
    public function setMissing(?array $missing): self
    {
        $this->initialized['missing'] = true;
        $this->missing = $missing;
        return $this;
    }
}