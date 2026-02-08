<?php

namespace FlowCatalyst\Generated\Model;

class OauthTokenPostBody extends \ArrayObject
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
    protected $grantType;
    /**
     * @var string|null
     */
    protected $code;
    /**
     * @var string|null
     */
    protected $redirectUri;
    /**
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $clientSecret;
    /**
     * @var string|null
     */
    protected $codeVerifier;
    /**
     * @var string|null
     */
    protected $refreshToken;
    /**
     * @var string|null
     */
    protected $username;
    /**
     * @var string|null
     */
    protected $password;
    /**
     * @var string|null
     */
    protected $scope;
    /**
     * @return string|null
     */
    public function getGrantType(): ?string
    {
        return $this->grantType;
    }
    /**
     * @param string|null $grantType
     *
     * @return self
     */
    public function setGrantType(?string $grantType): self
    {
        $this->initialized['grantType'] = true;
        $this->grantType = $grantType;
        return $this;
    }
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
    public function getRedirectUri(): ?string
    {
        return $this->redirectUri;
    }
    /**
     * @param string|null $redirectUri
     *
     * @return self
     */
    public function setRedirectUri(?string $redirectUri): self
    {
        $this->initialized['redirectUri'] = true;
        $this->redirectUri = $redirectUri;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * @param string|null $clientId
     *
     * @return self
     */
    public function setClientId(?string $clientId): self
    {
        $this->initialized['clientId'] = true;
        $this->clientId = $clientId;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getClientSecret(): ?string
    {
        return $this->clientSecret;
    }
    /**
     * @param string|null $clientSecret
     *
     * @return self
     */
    public function setClientSecret(?string $clientSecret): self
    {
        $this->initialized['clientSecret'] = true;
        $this->clientSecret = $clientSecret;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getCodeVerifier(): ?string
    {
        return $this->codeVerifier;
    }
    /**
     * @param string|null $codeVerifier
     *
     * @return self
     */
    public function setCodeVerifier(?string $codeVerifier): self
    {
        $this->initialized['codeVerifier'] = true;
        $this->codeVerifier = $codeVerifier;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRefreshToken(): ?string
    {
        return $this->refreshToken;
    }
    /**
     * @param string|null $refreshToken
     *
     * @return self
     */
    public function setRefreshToken(?string $refreshToken): self
    {
        $this->initialized['refreshToken'] = true;
        $this->refreshToken = $refreshToken;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getUsername(): ?string
    {
        return $this->username;
    }
    /**
     * @param string|null $username
     *
     * @return self
     */
    public function setUsername(?string $username): self
    {
        $this->initialized['username'] = true;
        $this->username = $username;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPassword(): ?string
    {
        return $this->password;
    }
    /**
     * @param string|null $password
     *
     * @return self
     */
    public function setPassword(?string $password): self
    {
        $this->initialized['password'] = true;
        $this->password = $password;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
     * @param string|null $scope
     *
     * @return self
     */
    public function setScope(?string $scope): self
    {
        $this->initialized['scope'] = true;
        $this->scope = $scope;
        return $this;
    }
}