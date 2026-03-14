<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminRolesPostBody extends \ArrayObject
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
     * @var mixed|null
     */
    protected $applicationId;
    /**
     * @var mixed|null
     */
    protected $applicationCode;
    /**
     * @var string|null
     */
    protected $shortName;
    /**
     * @var string|null
     */
    protected $displayName;
    /**
     * @var mixed|null
     */
    protected $description;
    /**
     * @var mixed|null
     */
    protected $source;
    /**
     * @var list<string>|null
     */
    protected $permissions;
    /**
     * @var bool|null
     */
    protected $clientManaged;
    /**
     * @return mixed
     */
    public function getApplicationId()
    {
        return $this->applicationId;
    }
    /**
     * @param mixed $applicationId
     *
     * @return self
     */
    public function setApplicationId($applicationId): self
    {
        $this->initialized['applicationId'] = true;
        $this->applicationId = $applicationId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getApplicationCode()
    {
        return $this->applicationCode;
    }
    /**
     * @param mixed $applicationCode
     *
     * @return self
     */
    public function setApplicationCode($applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getShortName(): ?string
    {
        return $this->shortName;
    }
    /**
     * @param string|null $shortName
     *
     * @return self
     */
    public function setShortName(?string $shortName): self
    {
        $this->initialized['shortName'] = true;
        $this->shortName = $shortName;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDisplayName(): ?string
    {
        return $this->displayName;
    }
    /**
     * @param string|null $displayName
     *
     * @return self
     */
    public function setDisplayName(?string $displayName): self
    {
        $this->initialized['displayName'] = true;
        $this->displayName = $displayName;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getDescription()
    {
        return $this->description;
    }
    /**
     * @param mixed $description
     *
     * @return self
     */
    public function setDescription($description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSource()
    {
        return $this->source;
    }
    /**
     * @param mixed $source
     *
     * @return self
     */
    public function setSource($source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getPermissions(): ?array
    {
        return $this->permissions;
    }
    /**
     * @param list<string>|null $permissions
     *
     * @return self
     */
    public function setPermissions(?array $permissions): self
    {
        $this->initialized['permissions'] = true;
        $this->permissions = $permissions;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getClientManaged(): ?bool
    {
        return $this->clientManaged;
    }
    /**
     * @param bool|null $clientManaged
     *
     * @return self
     */
    public function setClientManaged(?bool $clientManaged): self
    {
        $this->initialized['clientManaged'] = true;
        $this->clientManaged = $clientManaged;
        return $this;
    }
}