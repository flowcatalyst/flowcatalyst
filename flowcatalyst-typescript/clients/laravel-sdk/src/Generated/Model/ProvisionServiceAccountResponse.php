<?php

namespace FlowCatalyst\Generated\Model;

class ProvisionServiceAccountResponse extends \ArrayObject
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
     * Status message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Service account information
     *
     * @var ServiceAccountInfo|null
     */
    protected $serviceAccount;
    /**
     * Status message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Status message
     *
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
    /**
     * Service account information
     *
     * @return ServiceAccountInfo|null
     */
    public function getServiceAccount(): ?ServiceAccountInfo
    {
        return $this->serviceAccount;
    }
    /**
     * Service account information
     *
     * @param ServiceAccountInfo|null $serviceAccount
     *
     * @return self
     */
    public function setServiceAccount(?ServiceAccountInfo $serviceAccount): self
    {
        $this->initialized['serviceAccount'] = true;
        $this->serviceAccount = $serviceAccount;
        return $this;
    }
}