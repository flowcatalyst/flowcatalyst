<?php

namespace FlowCatalyst\Generated\Model;

class PushSyncResponse
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
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var PushSyncCounts|null
     */
    protected $keys;
    /**
     * @var PushSyncCounts|null
     */
    protected $rules;
    /**
     * @var list<string>|null
     */
    protected $syncedKids;
    /**
     * @var list<string>|null
     */
    protected $syncedPatterns;
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
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return PushSyncCounts|null
     */
    public function getKeys(): ?PushSyncCounts
    {
        return $this->keys;
    }
    /**
     * @param PushSyncCounts|null $keys
     *
     * @return self
     */
    public function setKeys(?PushSyncCounts $keys): self
    {
        $this->initialized['keys'] = true;
        $this->keys = $keys;
        return $this;
    }
    /**
     * @return PushSyncCounts|null
     */
    public function getRules(): ?PushSyncCounts
    {
        return $this->rules;
    }
    /**
     * @param PushSyncCounts|null $rules
     *
     * @return self
     */
    public function setRules(?PushSyncCounts $rules): self
    {
        $this->initialized['rules'] = true;
        $this->rules = $rules;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getSyncedKids(): ?array
    {
        return $this->syncedKids;
    }
    /**
     * @param list<string>|null $syncedKids
     *
     * @return self
     */
    public function setSyncedKids(?array $syncedKids): self
    {
        $this->initialized['syncedKids'] = true;
        $this->syncedKids = $syncedKids;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getSyncedPatterns(): ?array
    {
        return $this->syncedPatterns;
    }
    /**
     * @param list<string>|null $syncedPatterns
     *
     * @return self
     */
    public function setSyncedPatterns(?array $syncedPatterns): self
    {
        $this->initialized['syncedPatterns'] = true;
        $this->syncedPatterns = $syncedPatterns;
        return $this;
    }
}