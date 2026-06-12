<?php

namespace FlowCatalyst\Generated\Model;

class CreateIdentityProviderRequest extends \ArrayObject
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
     * @var list<string>|null
     */
    protected $allowedEmailDomains;
    /**
     * IDP code (e.g. internal, entra)
     *
     * @var string|null
     */
    protected $code;
    /**
     * Display name
     *
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $oidcClientId;
    /**
     * @var string|null
     */
    protected $oidcClientSecretRef;
    /**
     * @var string|null
     */
    protected $oidcIssuerPattern;
    /**
     * @var string|null
     */
    protected $oidcIssuerUrl;
    /**
     * @var bool|null
     */
    protected $oidcMultiTenant;
    /**
     * IDP type (INTERNAL or OIDC)
     *
     * @var string|null
     */
    protected $type;
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
    /**
     * IDP code (e.g. internal, entra)
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * IDP code (e.g. internal, entra)
     *
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
     * Display name
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * Display name
     *
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
     * @return string|null
     */
    public function getOidcClientSecretRef(): ?string
    {
        return $this->oidcClientSecretRef;
    }
    /**
     * @param string|null $oidcClientSecretRef
     *
     * @return self
     */
    public function setOidcClientSecretRef(?string $oidcClientSecretRef): self
    {
        $this->initialized['oidcClientSecretRef'] = true;
        $this->oidcClientSecretRef = $oidcClientSecretRef;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOidcIssuerPattern(): ?string
    {
        return $this->oidcIssuerPattern;
    }
    /**
     * @param string|null $oidcIssuerPattern
     *
     * @return self
     */
    public function setOidcIssuerPattern(?string $oidcIssuerPattern): self
    {
        $this->initialized['oidcIssuerPattern'] = true;
        $this->oidcIssuerPattern = $oidcIssuerPattern;
        return $this;
    }
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
     * IDP type (INTERNAL or OIDC)
     *
     * @return string|null
     */
    public function getType(): ?string
    {
        return $this->type;
    }
    /**
     * IDP type (INTERNAL or OIDC)
     *
     * @param string|null $type
     *
     * @return self
     */
    public function setType(?string $type): self
    {
        $this->initialized['type'] = true;
        $this->type = $type;
        return $this;
    }
}