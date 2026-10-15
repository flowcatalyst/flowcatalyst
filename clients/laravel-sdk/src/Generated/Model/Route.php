<?php

namespace FlowCatalyst\Generated\Model;

class Route
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
     * @var string|null
     */
    protected $address;
    /**
     * @var string|null
     */
    protected $alias;
    /**
     * @var list<string>|null
     */
    protected $aliasPrefixes;
    /**
     * @var string|null
     */
    protected $hostname;
    /**
     * @var string|null
     */
    protected $pathPrefix;
    /**
     * @return string|null
     */
    public function getAddress(): ?string
    {
        return $this->address;
    }
    /**
     * @param string|null $address
     *
     * @return self
     */
    public function setAddress(?string $address): self
    {
        $this->initialized['address'] = true;
        $this->address = $address;
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
}