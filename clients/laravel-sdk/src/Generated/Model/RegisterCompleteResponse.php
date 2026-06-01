<?php

namespace FlowCatalyst\Generated\Model;

class RegisterCompleteResponse extends \ArrayObject
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
    protected $credentialId;
    /**
     * @return string|null
     */
    public function getCredentialId(): ?string
    {
        return $this->credentialId;
    }
    /**
     * @param string|null $credentialId
     *
     * @return self
     */
    public function setCredentialId(?string $credentialId): self
    {
        $this->initialized['credentialId'] = true;
        $this->credentialId = $credentialId;
        return $this;
    }
}