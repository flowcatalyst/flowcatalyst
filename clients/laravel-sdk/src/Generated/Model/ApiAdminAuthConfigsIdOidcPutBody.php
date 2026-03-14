<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminAuthConfigsIdOidcPutBody extends \ArrayObject
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
    protected $oidcIssuerUrl;
    /**
     * @var string|null
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
     * @return string|null
     */
    public function getOidcIssuerUrl(): ?string
    {
        return $this->oidcIssuerUrl;
    }
    /**
     * @param string|null $oidcIssuerUrl
     *
     * @return self
     */
    public function setOidcIssuerUrl(?string $oidcIssuerUrl): self
    {
        $this->initialized['oidcIssuerUrl'] = true;
        $this->oidcIssuerUrl = $oidcIssuerUrl;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOidcClientId(): ?string
    {
        return $this->oidcClientId;
    }
    /**
     * @param string|null $oidcClientId
     *
     * @return self
     */
    public function setOidcClientId(?string $oidcClientId): self
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
}