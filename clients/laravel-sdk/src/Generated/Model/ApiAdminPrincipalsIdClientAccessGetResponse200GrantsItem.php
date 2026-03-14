<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPrincipalsIdClientAccessGetResponse200GrantsItem extends \ArrayObject
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
    protected $clientId;
    /**
     * @var string|null
     */
    protected $grantedBy;
    /**
     * @var \DateTime|null
     */
    protected $grantedAt;
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
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getGrantedBy(): ?string
    {
        return $this->grantedBy;
    }
    /**
     * @param string|null $grantedBy
     *
     * @return self
     */
    public function setGrantedBy(?string $grantedBy): self
    {
        $this->initialized['grantedBy'] = true;
        $this->grantedBy = $grantedBy;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getGrantedAt(): ?\DateTime
    {
        return $this->grantedAt;
    }
    /**
     * @param \DateTime|null $grantedAt
     *
     * @return self
     */
    public function setGrantedAt(?\DateTime $grantedAt): self
    {
        $this->initialized['grantedAt'] = true;
        $this->grantedAt = $grantedAt;
        return $this;
    }
}