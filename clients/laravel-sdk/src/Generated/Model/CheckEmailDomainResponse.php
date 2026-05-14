<?php

namespace FlowCatalyst\Generated\Model;

class CheckEmailDomainResponse extends \ArrayObject
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
     * When the user must pick a client, this is the allow-list to choose
     * from. Empty when `requiresClientId` is false (no input needed) OR
     * when there is no per-domain restriction (any active client is valid —
     * the UI shows the full list it already fetches).
     *
     * @var list<string>|null
     */
    protected $allowedClientIds;
    /**
     * Auth provider if configured (INTERNAL, OIDC)
     *
     * @var string|null
     */
    protected $authProvider;
    /**
     * Scope the user will be created with (ANCHOR / PARTNER / CLIENT).
     * Derived from anchor domains + email_domain_mappings; unmapped domains
     * default to CLIENT.
     *
     * @var string|null
     */
    protected $derivedScope;
    /**
     * The domain that was checked
     *
     * @var string|null
     */
    protected $domain;
    /**
     * Whether the email already exists
     *
     * @var bool|null
     */
    protected $emailExists;
    /**
     * Whether this domain has auth configuration
     *
     * @var bool|null
     */
    protected $hasAuthConfig;
    /**
     * Informational message
     *
     * @var string|null
     */
    protected $info;
    /**
     * Whether this is an anchor domain
     *
     * @var bool|null
     */
    protected $isAnchorDomain;
    /**
     * True when the create-user form must supply a `clientId`. False for
     * anchor domains and for mappings that already pin a primary client.
     *
     * @var bool|null
     */
    protected $requiresClientId;
    /**
     * Warning message
     *
     * @var string|null
     */
    protected $warning;
    /**
     * When the user must pick a client, this is the allow-list to choose
     * from. Empty when `requiresClientId` is false (no input needed) OR
     * when there is no per-domain restriction (any active client is valid —
     * the UI shows the full list it already fetches).
     *
     * @return list<string>|null
     */
    public function getAllowedClientIds(): ?array
    {
        return $this->allowedClientIds;
    }
    /**
    * When the user must pick a client, this is the allow-list to choose
    from. Empty when `requiresClientId` is false (no input needed) OR
    when there is no per-domain restriction (any active client is valid —
    the UI shows the full list it already fetches).
    *
    * @param list<string>|null $allowedClientIds
    *
    * @return self
    */
    public function setAllowedClientIds(?array $allowedClientIds): self
    {
        $this->initialized['allowedClientIds'] = true;
        $this->allowedClientIds = $allowedClientIds;
        return $this;
    }
    /**
     * Auth provider if configured (INTERNAL, OIDC)
     *
     * @return string|null
     */
    public function getAuthProvider(): ?string
    {
        return $this->authProvider;
    }
    /**
     * Auth provider if configured (INTERNAL, OIDC)
     *
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
     * Scope the user will be created with (ANCHOR / PARTNER / CLIENT).
     * Derived from anchor domains + email_domain_mappings; unmapped domains
     * default to CLIENT.
     *
     * @return string|null
     */
    public function getDerivedScope(): ?string
    {
        return $this->derivedScope;
    }
    /**
    * Scope the user will be created with (ANCHOR / PARTNER / CLIENT).
    Derived from anchor domains + email_domain_mappings; unmapped domains
    default to CLIENT.
    *
    * @param string|null $derivedScope
    *
    * @return self
    */
    public function setDerivedScope(?string $derivedScope): self
    {
        $this->initialized['derivedScope'] = true;
        $this->derivedScope = $derivedScope;
        return $this;
    }
    /**
     * The domain that was checked
     *
     * @return string|null
     */
    public function getDomain(): ?string
    {
        return $this->domain;
    }
    /**
     * The domain that was checked
     *
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
     * Whether the email already exists
     *
     * @return bool|null
     */
    public function getEmailExists(): ?bool
    {
        return $this->emailExists;
    }
    /**
     * Whether the email already exists
     *
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
     * Whether this domain has auth configuration
     *
     * @return bool|null
     */
    public function getHasAuthConfig(): ?bool
    {
        return $this->hasAuthConfig;
    }
    /**
     * Whether this domain has auth configuration
     *
     * @param bool|null $hasAuthConfig
     *
     * @return self
     */
    public function setHasAuthConfig(?bool $hasAuthConfig): self
    {
        $this->initialized['hasAuthConfig'] = true;
        $this->hasAuthConfig = $hasAuthConfig;
        return $this;
    }
    /**
     * Informational message
     *
     * @return string|null
     */
    public function getInfo(): ?string
    {
        return $this->info;
    }
    /**
     * Informational message
     *
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
     * Whether this is an anchor domain
     *
     * @return bool|null
     */
    public function getIsAnchorDomain(): ?bool
    {
        return $this->isAnchorDomain;
    }
    /**
     * Whether this is an anchor domain
     *
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
     * True when the create-user form must supply a `clientId`. False for
     * anchor domains and for mappings that already pin a primary client.
     *
     * @return bool|null
     */
    public function getRequiresClientId(): ?bool
    {
        return $this->requiresClientId;
    }
    /**
    * True when the create-user form must supply a `clientId`. False for
    anchor domains and for mappings that already pin a primary client.
    *
    * @param bool|null $requiresClientId
    *
    * @return self
    */
    public function setRequiresClientId(?bool $requiresClientId): self
    {
        $this->initialized['requiresClientId'] = true;
        $this->requiresClientId = $requiresClientId;
        return $this;
    }
    /**
     * Warning message
     *
     * @return string|null
     */
    public function getWarning(): ?string
    {
        return $this->warning;
    }
    /**
     * Warning message
     *
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