<?php

namespace FlowCatalyst\Generated\Model;

class PublicationResponse
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
     * Whether this platform is configured to publish credentials at all
     *
     * @var bool|null
     */
    protected $available;
    /**
     * @var string|null
     */
    protected $basePath;
    /**
     * @var bool|null
     */
    protected $enabled;
    /**
     * @var \DateTime|null
     */
    protected $enabledAt;
    /**
     * @var string|null
     */
    protected $enabledBy;
    /**
     * @var list<ItemResponse>|null
     */
    protected $items;
    /**
     * OAuth clients only: set when the client belongs to a service account, whose publication carries its credentials
     *
     * @var string|null
     */
    protected $linkedServiceAccountId;
    /**
     * @var string|null
     */
    protected $store;
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
     * Whether this platform is configured to publish credentials at all
     *
     * @return bool|null
     */
    public function getAvailable(): ?bool
    {
        return $this->available;
    }
    /**
     * Whether this platform is configured to publish credentials at all
     *
     * @param bool|null $available
     *
     * @return self
     */
    public function setAvailable(?bool $available): self
    {
        $this->initialized['available'] = true;
        $this->available = $available;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getBasePath(): ?string
    {
        return $this->basePath;
    }
    /**
     * @param string|null $basePath
     *
     * @return self
     */
    public function setBasePath(?string $basePath): self
    {
        $this->initialized['basePath'] = true;
        $this->basePath = $basePath;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getEnabled(): ?bool
    {
        return $this->enabled;
    }
    /**
     * @param bool|null $enabled
     *
     * @return self
     */
    public function setEnabled(?bool $enabled): self
    {
        $this->initialized['enabled'] = true;
        $this->enabled = $enabled;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getEnabledAt(): ?\DateTime
    {
        return $this->enabledAt;
    }
    /**
     * @param \DateTime|null $enabledAt
     *
     * @return self
     */
    public function setEnabledAt(?\DateTime $enabledAt): self
    {
        $this->initialized['enabledAt'] = true;
        $this->enabledAt = $enabledAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEnabledBy(): ?string
    {
        return $this->enabledBy;
    }
    /**
     * @param string|null $enabledBy
     *
     * @return self
     */
    public function setEnabledBy(?string $enabledBy): self
    {
        $this->initialized['enabledBy'] = true;
        $this->enabledBy = $enabledBy;
        return $this;
    }
    /**
     * @return list<ItemResponse>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<ItemResponse>|null $items
     *
     * @return self
     */
    public function setItems(?array $items): self
    {
        $this->initialized['items'] = true;
        $this->items = $items;
        return $this;
    }
    /**
     * OAuth clients only: set when the client belongs to a service account, whose publication carries its credentials
     *
     * @return string|null
     */
    public function getLinkedServiceAccountId(): ?string
    {
        return $this->linkedServiceAccountId;
    }
    /**
     * OAuth clients only: set when the client belongs to a service account, whose publication carries its credentials
     *
     * @param string|null $linkedServiceAccountId
     *
     * @return self
     */
    public function setLinkedServiceAccountId(?string $linkedServiceAccountId): self
    {
        $this->initialized['linkedServiceAccountId'] = true;
        $this->linkedServiceAccountId = $linkedServiceAccountId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStore(): ?string
    {
        return $this->store;
    }
    /**
     * @param string|null $store
     *
     * @return self
     */
    public function setStore(?string $store): self
    {
        $this->initialized['store'] = true;
        $this->store = $store;
        return $this;
    }
}