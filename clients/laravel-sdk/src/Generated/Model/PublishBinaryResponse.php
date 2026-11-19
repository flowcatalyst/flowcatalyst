<?php

namespace FlowCatalyst\Generated\Model;

class PublishBinaryResponse
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
     * @var AppBinaryResponse|null
     */
    protected $binary;
    /**
     * false when this file was already published to this channel and variant (nothing changed)
     *
     * @var bool|null
     */
    protected $created;
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
     * @return AppBinaryResponse|null
     */
    public function getBinary(): ?AppBinaryResponse
    {
        return $this->binary;
    }
    /**
     * @param AppBinaryResponse|null $binary
     *
     * @return self
     */
    public function setBinary(?AppBinaryResponse $binary): self
    {
        $this->initialized['binary'] = true;
        $this->binary = $binary;
        return $this;
    }
    /**
     * false when this file was already published to this channel and variant (nothing changed)
     *
     * @return bool|null
     */
    public function getCreated(): ?bool
    {
        return $this->created;
    }
    /**
     * false when this file was already published to this channel and variant (nothing changed)
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
}