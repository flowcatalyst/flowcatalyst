<?php

namespace FlowCatalyst\Generated\Model;

class ResendInvitationRequest extends \ArrayObject
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
    protected $inviteRedirectUri;
    /**
     * @var bool|null
     */
    protected $returnInviteLink;
    /**
     * @var bool|null
     */
    protected $sendInvitation;
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
    public function getInviteRedirectUri(): ?string
    {
        return $this->inviteRedirectUri;
    }
    /**
     * @param string|null $inviteRedirectUri
     *
     * @return self
     */
    public function setInviteRedirectUri(?string $inviteRedirectUri): self
    {
        $this->initialized['inviteRedirectUri'] = true;
        $this->inviteRedirectUri = $inviteRedirectUri;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getReturnInviteLink(): ?bool
    {
        return $this->returnInviteLink;
    }
    /**
     * @param bool|null $returnInviteLink
     *
     * @return self
     */
    public function setReturnInviteLink(?bool $returnInviteLink): self
    {
        $this->initialized['returnInviteLink'] = true;
        $this->returnInviteLink = $returnInviteLink;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getSendInvitation(): ?bool
    {
        return $this->sendInvitation;
    }
    /**
     * @param bool|null $sendInvitation
     *
     * @return self
     */
    public function setSendInvitation(?bool $sendInvitation): self
    {
        $this->initialized['sendInvitation'] = true;
        $this->sendInvitation = $sendInvitation;
        return $this;
    }
}