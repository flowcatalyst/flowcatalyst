<?php

namespace FlowCatalyst\Generated\Model;

class UpdateRoleRequest extends \ArrayObject
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
     * Display name
     *
     * @var string|null
     */
    protected $displayName;
    /**
     * Role description
     *
     * @var string|null
     */
    protected $description;
    /**
     * Structured permission inputs
     *
     * @var list<PermissionInputDto>|null
     */
    protected $permissions;
    /**
     * Whether clients can manage assignment
     *
     * @var bool|null
     */
    protected $clientManaged;
    /**
     * Display name
     *
     * @return string|null
     */
    public function getDisplayName(): ?string
    {
        return $this->displayName;
    }
    /**
     * Display name
     *
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
     * Role description
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Role description
     *
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * Structured permission inputs
     *
     * @return list<PermissionInputDto>|null
     */
    public function getPermissions(): ?array
    {
        return $this->permissions;
    }
    /**
     * Structured permission inputs
     *
     * @param list<PermissionInputDto>|null $permissions
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
     * Whether clients can manage assignment
     *
     * @return bool|null
     */
    public function getClientManaged(): ?bool
    {
        return $this->clientManaged;
    }
    /**
     * Whether clients can manage assignment
     *
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