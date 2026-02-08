<?php

namespace FlowCatalyst\Generated\Model;

class EmailDomainCheckResponse extends \ArrayObject
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
    protected $domain;
    /**
     * @var string|null
     */
    protected $authProvider;
    /**
     * @var bool|null
     */
    protected $isAnchorDomain;
    /**
     * @var bool|null
     */
    protected $hasIdpConfig;
    /**
     * @var bool|null
     */
    protected $emailExists;
    /**
     * @var string|null
     */
    protected $info;
    /**
     * @var string|null
     */
    protected $warning;
    /**
     * @return string|null
     */
    public function getDomain(): ?string
    {
        return $this->domain;
    }
    /**
     * @param string|null $domain
     *
     * @return self
     */
    public function setDomain(?string $domain): self
    {
        $this->initialized['domain'] = true;
        $this->domain = $domain;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAuthProvider(): ?string
    {
        return $this->authProvider;
    }
    /**
     * @param string|null $authProvider
     *
     * @return self
     */
    public function setAuthProvider(?string $authProvider): self
    {
        $this->initialized['authProvider'] = true;
        $this->authProvider = $authProvider;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getIsAnchorDomain(): ?bool
    {
        return $this->isAnchorDomain;
    }
    /**
     * @param bool|null $isAnchorDomain
     *
     * @return self
     */
    public function setIsAnchorDomain(?bool $isAnchorDomain): self
    {
        $this->initialized['isAnchorDomain'] = true;
        $this->isAnchorDomain = $isAnchorDomain;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getHasIdpConfig(): ?bool
    {
        return $this->hasIdpConfig;
    }
    /**
     * @param bool|null $hasIdpConfig
     *
     * @return self
     */
    public function setHasIdpConfig(?bool $hasIdpConfig): self
    {
        $this->initialized['hasIdpConfig'] = true;
        $this->hasIdpConfig = $hasIdpConfig;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getEmailExists(): ?bool
    {
        return $this->emailExists;
    }
    /**
     * @param bool|null $emailExists
     *
     * @return self
     */
    public function setEmailExists(?bool $emailExists): self
    {
        $this->initialized['emailExists'] = true;
        $this->emailExists = $emailExists;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getInfo(): ?string
    {
        return $this->info;
    }
    /**
     * @param string|null $info
     *
     * @return self
     */
    public function setInfo(?string $info): self
    {
        $this->initialized['info'] = true;
        $this->info = $info;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getWarning(): ?string
    {
        return $this->warning;
    }
    /**
     * @param string|null $warning
     *
     * @return self
     */
    public function setWarning(?string $warning): self
    {
        $this->initialized['warning'] = true;
        $this->warning = $warning;
        return $this;
    }
}