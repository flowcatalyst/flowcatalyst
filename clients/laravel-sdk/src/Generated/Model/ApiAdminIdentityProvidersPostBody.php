<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminIdentityProvidersPostBody extends \ArrayObject
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
    protected $code;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var mixed|null
     */
    protected $type;
    /**
     * @var mixed|null
     */
    protected $oidcIssuerUrl;
    /**
     * @var mixed|null
     */
    protected $oidcClientId;
    /**
     * @var mixed|null
     */
    protected $oidcClientSecretRef;
    /**
     * @var bool|null
     */
    protected $oidcMultiTenant;
    /**
     * @var mixed|null
     */
    protected $oidcIssuerPattern;
    /**
     * @var list<string>|null
     */
    protected $allowedEmailDomains;
    /**
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getType()
    {
        return $this->type;
    }
    /**
     * @param mixed $type
     *
     * @return self
     */
    public function setType($type): self
    {
        $this->initialized['type'] = true;
        $this->type = $type;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcIssuerUrl()
    {
        return $this->oidcIssuerUrl;
    }
    /**
     * @param mixed $oidcIssuerUrl
     *
     * @return self
     */
    public function setOidcIssuerUrl($oidcIssuerUrl): self
    {
        $this->initialized['oidcIssuerUrl'] = true;
        $this->oidcIssuerUrl = $oidcIssuerUrl;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcClientId()
    {
        return $this->oidcClientId;
    }
    /**
     * @param mixed $oidcClientId
     *
     * @return self
     */
    public function setOidcClientId($oidcClientId): self
    {
        $this->initialized['oidcClientId'] = true;
        $this->oidcClientId = $oidcClientId;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcClientSecretRef()
    {
        return $this->oidcClientSecretRef;
    }
    /**
     * @param mixed $oidcClientSecretRef
     *
     * @return self
     */
    public function setOidcClientSecretRef($oidcClientSecretRef): self
    {
        $this->initialized['oidcClientSecretRef'] = true;
        $this->oidcClientSecretRef = $oidcClientSecretRef;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getOidcMultiTenant(): ?bool
    {
        return $this->oidcMultiTenant;
    }
    /**
     * @param bool|null $oidcMultiTenant
     *
     * @return self
     */
    public function setOidcMultiTenant(?bool $oidcMultiTenant): self
    {
        $this->initialized['oidcMultiTenant'] = true;
        $this->oidcMultiTenant = $oidcMultiTenant;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getOidcIssuerPattern()
    {
        return $this->oidcIssuerPattern;
    }
    /**
     * @param mixed $oidcIssuerPattern
     *
     * @return self
     */
    public function setOidcIssuerPattern($oidcIssuerPattern): self
    {
        $this->initialized['oidcIssuerPattern'] = true;
        $this->oidcIssuerPattern = $oidcIssuerPattern;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getAllowedEmailDomains(): ?array
    {
        return $this->allowedEmailDomains;
    }
    /**
     * @param list<string>|null $allowedEmailDomains
     *
     * @return self
     */
    public function setAllowedEmailDomains(?array $allowedEmailDomains): self
    {
        $this->initialized['allowedEmailDomains'] = true;
        $this->allowedEmailDomains = $allowedEmailDomains;
        return $this;
    }
}