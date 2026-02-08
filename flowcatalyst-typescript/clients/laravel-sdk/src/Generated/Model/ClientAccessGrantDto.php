<?php

namespace FlowCatalyst\Generated\Model;

class ClientAccessGrantDto extends \ArrayObject
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
     * Grant ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Client ID
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * @var \DateTime|null
     */
    protected $grantedAt;
    /**
     * @var \DateTime|null
     */
    protected $expiresAt;
    /**
     * Grant ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Grant ID
     *
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
     * Client ID
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * Client ID
     *
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
    /**
     * @return \DateTime|null
     */
    public function getExpiresAt(): ?\DateTime
    {
        return $this->expiresAt;
    }
    /**
     * @param \DateTime|null $expiresAt
     *
     * @return self
     */
    public function setExpiresAt(?\DateTime $expiresAt): self
    {
        $this->initialized['expiresAt'] = true;
        $this->expiresAt = $expiresAt;
        return $this;
    }
}