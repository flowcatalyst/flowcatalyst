<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse200 extends \ArrayObject
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
    protected $signingSecret;
    /**
     * @return string|null
     */
    public function getSigningSecret(): ?string
    {
        return $this->signingSecret;
    }
    /**
     * @param string|null $signingSecret
     *
     * @return self
     */
    public function setSigningSecret(?string $signingSecret): self
    {
        $this->initialized['signingSecret'] = true;
        $this->signingSecret = $signingSecret;
        return $this;
    }
}