<?php

namespace FlowCatalyst\Generated\Model;

class RollbackResponse
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
     * false when the branch was already rolled back to the embedded bundle
     *
     * @var bool|null
     */
    protected $created;
    /**
     * @var AppUpdateResponse|null
     */
    protected $update;
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
     * false when the branch was already rolled back to the embedded bundle
     *
     * @return bool|null
     */
    public function getCreated(): ?bool
    {
        return $this->created;
    }
    /**
     * false when the branch was already rolled back to the embedded bundle
     *
     * @param bool|null $created
     *
     * @return self
     */
    public function setCreated(?bool $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
        return $this;
    }
    /**
     * @return AppUpdateResponse|null
     */
    public function getUpdate(): ?AppUpdateResponse
    {
        return $this->update;
    }
    /**
     * @param AppUpdateResponse|null $update
     *
     * @return self
     */
    public function setUpdate(?AppUpdateResponse $update): self
    {
        $this->initialized['update'] = true;
        $this->update = $update;
        return $this;
    }
}