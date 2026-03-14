<?php

namespace FlowCatalyst\Generated\Model;

class ApiSdkPrincipalsIdRolesGetResponse200RolesItem extends \ArrayObject
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
    protected $roleName;
    /**
     * @var string|null
     */
    protected $assignmentSource;
    /**
     * @var \DateTime|null
     */
    protected $assignedAt;
    /**
     * @return string|null
     */
    public function getRoleName(): ?string
    {
        return $this->roleName;
    }
    /**
     * @param string|null $roleName
     *
     * @return self
     */
    public function setRoleName(?string $roleName): self
    {
        $this->initialized['roleName'] = true;
        $this->roleName = $roleName;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAssignmentSource(): ?string
    {
        return $this->assignmentSource;
    }
    /**
     * @param string|null $assignmentSource
     *
     * @return self
     */
    public function setAssignmentSource(?string $assignmentSource): self
    {
        $this->initialized['assignmentSource'] = true;
        $this->assignmentSource = $assignmentSource;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getAssignedAt(): ?\DateTime
    {
        return $this->assignedAt;
    }
    /**
     * @param \DateTime|null $assignedAt
     *
     * @return self
     */
    public function setAssignedAt(?\DateTime $assignedAt): self
    {
        $this->initialized['assignedAt'] = true;
        $this->assignedAt = $assignedAt;
        return $this;
    }
}