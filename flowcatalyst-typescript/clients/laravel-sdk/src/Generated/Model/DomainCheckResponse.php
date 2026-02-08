<?php

namespace FlowCatalyst\Generated\Model;

class DomainCheckResponse extends \ArrayObject
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
    protected $authMethod;
    /**
     * @var string|null
     */
    protected $loginUrl;
    /**
     * @var string|null
     */
    protected $idpIssuer;
    /**
     * @return string|null
     */
    public function getAuthMethod(): ?string
    {
        return $this->authMethod;
    }
    /**
     * @param string|null $authMethod
     *
     * @return self
     */
    public function setAuthMethod(?string $authMethod): self
    {
        $this->initialized['authMethod'] = true;
        $this->authMethod = $authMethod;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getLoginUrl(): ?string
    {
        return $this->loginUrl;
    }
    /**
     * @param string|null $loginUrl
     *
     * @return self
     */
    public function setLoginUrl(?string $loginUrl): self
    {
        $this->initialized['loginUrl'] = true;
        $this->loginUrl = $loginUrl;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIdpIssuer(): ?string
    {
        return $this->idpIssuer;
    }
    /**
     * @param string|null $idpIssuer
     *
     * @return self
     */
    public function setIdpIssuer(?string $idpIssuer): self
    {
        $this->initialized['idpIssuer'] = true;
        $this->idpIssuer = $idpIssuer;
        return $this;
    }
}