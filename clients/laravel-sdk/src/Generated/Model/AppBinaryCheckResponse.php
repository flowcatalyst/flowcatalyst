<?php

namespace FlowCatalyst\Generated\Model;

class AppBinaryCheckResponse
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
     * @var int|null
     */
    protected $count;
    /**
     * @var AppBinaryResponse|null
     */
    protected $newest;
    /**
     * A binary of this runtime version is published on the channel (any variant)
     *
     * @var bool|null
     */
    protected $published;
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
     * @return int|null
     */
    public function getCount(): ?int
    {
        return $this->count;
    }
    /**
     * @param int|null $count
     *
     * @return self
     */
    public function setCount(?int $count): self
    {
        $this->initialized['count'] = true;
        $this->count = $count;
        return $this;
    }
    /**
     * @return AppBinaryResponse|null
     */
    public function getNewest(): ?AppBinaryResponse
    {
        return $this->newest;
    }
    /**
     * @param AppBinaryResponse|null $newest
     *
     * @return self
     */
    public function setNewest(?AppBinaryResponse $newest): self
    {
        $this->initialized['newest'] = true;
        $this->newest = $newest;
        return $this;
    }
    /**
     * A binary of this runtime version is published on the channel (any variant)
     *
     * @return bool|null
     */
    public function getPublished(): ?bool
    {
        return $this->published;
    }
    /**
     * A binary of this runtime version is published on the channel (any variant)
     *
     * @param bool|null $published
     *
     * @return self
     */
    public function setPublished(?bool $published): self
    {
        $this->initialized['published'] = true;
        $this->published = $published;
        return $this;
    }
}