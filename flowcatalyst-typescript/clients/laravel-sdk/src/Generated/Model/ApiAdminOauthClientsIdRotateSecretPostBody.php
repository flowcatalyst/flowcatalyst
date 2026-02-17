<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminOauthClientsIdRotateSecretPostBody extends \ArrayObject
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
    protected $newSecretRef;
    /**
     * @return string|null
     */
    public function getNewSecretRef(): ?string
    {
        return $this->newSecretRef;
    }
    /**
     * @param string|null $newSecretRef
     *
     * @return self
     */
    public function setNewSecretRef(?string $newSecretRef): self
    {
        $this->initialized['newSecretRef'] = true;
        $this->newSecretRef = $newSecretRef;
        return $this;
    }
}