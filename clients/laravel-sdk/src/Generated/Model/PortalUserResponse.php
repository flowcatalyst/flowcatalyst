<?php

namespace FlowCatalyst\Generated\Model;

class PortalUserResponse
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
     * @var bool|null
     */
    protected $created;
    /**
     * @var bool|null
     */
    protected $hasPassword;
    /**
     * @var string|null
     */
    protected $identityId;
    /**
     * @var string|null
     */
    protected $inviteUrl;
    /**
     * @var bool|null
     */
    protected $invited;
    /**
     * @var bool|null
     */
    protected $ssoManaged;
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
     * @return bool|null
     */
    public function getCreated(): ?bool
    {
        return $this->created;
    }
    /**
     * @param bool|null $created
     *
     * @return self
     */
    public function setCreated(?bool $created): self
    {
        $this->initialized['created'] = true;
        $this->created = $created;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getHasPassword(): ?bool
    {
        return $this->hasPassword;
    }
    /**
     * @param bool|null $hasPassword
     *
     * @return self
     */
    public function setHasPassword(?bool $hasPassword): self
    {
        $this->initialized['hasPassword'] = true;
        $this->hasPassword = $hasPassword;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdentityId(): ?string
    {
        return $this->identityId;
    }
    /**
     * @param string|null $identityId
     *
     * @return self
     */
    public function setIdentityId(?string $identityId): self
    {
        $this->initialized['identityId'] = true;
        $this->identityId = $identityId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getInviteUrl(): ?string
    {
        return $this->inviteUrl;
    }
    /**
     * @param string|null $inviteUrl
     *
     * @return self
     */
    public function setInviteUrl(?string $inviteUrl): self
    {
        $this->initialized['inviteUrl'] = true;
        $this->inviteUrl = $inviteUrl;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getInvited(): ?bool
    {
        return $this->invited;
    }
    /**
     * @param bool|null $invited
     *
     * @return self
     */
    public function setInvited(?bool $invited): self
    {
        $this->initialized['invited'] = true;
        $this->invited = $invited;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getSsoManaged(): ?bool
    {
        return $this->ssoManaged;
    }
    /**
     * @param bool|null $ssoManaged
     *
     * @return self
     */
    public function setSsoManaged(?bool $ssoManaged): self
    {
        $this->initialized['ssoManaged'] = true;
        $this->ssoManaged = $ssoManaged;
        return $this;
    }
}