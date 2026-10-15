<?php

namespace FlowCatalyst\Generated\Model;

class RouteResponse
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
    protected $alias;
    /**
     * @var list<string>|null
     */
    protected $aliasPrefixes;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var string|null
     */
    protected $createdBy;
    /**
     * @var string|null
     */
    protected $functionId;
    /**
     * @var string|null
     */
    protected $hostname;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $pathPrefix;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
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
    public function getAlias(): ?string
    {
        return $this->alias;
    }
    /**
     * @param string|null $alias
     *
     * @return self
     */
    public function setAlias(?string $alias): self
    {
        $this->initialized['alias'] = true;
        $this->alias = $alias;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAliasPrefixes(): ?array
    {
        return $this->aliasPrefixes;
    }
    /**
     * @param list<string>|null $aliasPrefixes
     *
     * @return self
     */
    public function setAliasPrefixes(?array $aliasPrefixes): self
    {
        $this->initialized['aliasPrefixes'] = true;
        $this->aliasPrefixes = $aliasPrefixes;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCreatedBy(): ?string
    {
        return $this->createdBy;
    }
    /**
     * @param string|null $createdBy
     *
     * @return self
     */
    public function setCreatedBy(?string $createdBy): self
    {
        $this->initialized['createdBy'] = true;
        $this->createdBy = $createdBy;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getFunctionId(): ?string
    {
        return $this->functionId;
    }
    /**
     * @param string|null $functionId
     *
     * @return self
     */
    public function setFunctionId(?string $functionId): self
    {
        $this->initialized['functionId'] = true;
        $this->functionId = $functionId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getHostname(): ?string
    {
        return $this->hostname;
    }
    /**
     * @param string|null $hostname
     *
     * @return self
     */
    public function setHostname(?string $hostname): self
    {
        $this->initialized['hostname'] = true;
        $this->hostname = $hostname;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
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
     * @return string|null
     */
    public function getPathPrefix(): ?string
    {
        return $this->pathPrefix;
    }
    /**
     * @param string|null $pathPrefix
     *
     * @return self
     */
    public function setPathPrefix(?string $pathPrefix): self
    {
        $this->initialized['pathPrefix'] = true;
        $this->pathPrefix = $pathPrefix;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}