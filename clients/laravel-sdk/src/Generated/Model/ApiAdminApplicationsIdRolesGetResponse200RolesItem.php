<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminApplicationsIdRolesGetResponse200RolesItem extends \ArrayObject
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
    protected $id;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $displayName;
    /**
     * @var mixed|null
     */
    protected $description;
    /**
     * @var list<string>|null
     */
    protected $permissions;
    /**
     * @var bool|null
     */
    protected $clientManaged;
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
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
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