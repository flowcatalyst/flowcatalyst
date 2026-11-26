<?php

namespace FlowCatalyst\Generated\Model;

class FieldUnlockKeyListResponse
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
     * @var list<FieldUnlockKeyResponse>|null
     */
    protected $unlockKeys;
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
     * @return list<FieldUnlockKeyResponse>|null
     */
    public function getUnlockKeys(): ?array
    {
        return $this->unlockKeys;
    }
    /**
     * @param list<FieldUnlockKeyResponse>|null $unlockKeys
     *
     * @return self
     */
    public function setUnlockKeys(?array $unlockKeys): self
    {
        $this->initialized['unlockKeys'] = true;
        $this->unlockKeys = $unlockKeys;
        return $this;
    }
}