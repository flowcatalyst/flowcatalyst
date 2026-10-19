<?php

namespace FlowCatalyst\Generated\Model;

class ResendInvitationResponse
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
     * @var \DateTime|null
     */
    protected $expiresAt;
    /**
     * @var string|null
     */
    protected $inviteLink;
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
    /**
     * @return string|null
     */
    public function getInviteLink(): ?string
    {
        return $this->inviteLink;
    }
    /**
     * @param string|null $inviteLink
     *
     * @return self
     */
    public function setInviteLink(?string $inviteLink): self
    {
        $this->initialized['inviteLink'] = true;
        $this->inviteLink = $inviteLink;
        return $this;
    }
}