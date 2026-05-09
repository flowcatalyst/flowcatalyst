<?php

namespace FlowCatalyst\Generated\Model;

class TokenRefreshResponse extends \ArrayObject
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
     * New access token
     *
     * @var string|null
     */
    protected $accessToken;
    /**
     * Expiration time in seconds
     *
     * @var int|null
     */
    protected $expiresIn;
    /**
     * New refresh token (rotation)
     *
     * @var string|null
     */
    protected $refreshToken;
    /**
     * Token type (always "Bearer")
     *
     * @var string|null
     */
    protected $tokenType;
    /**
     * New access token
     *
     * @return string|null
     */
    public function getAccessToken(): ?string
    {
        return $this->accessToken;
    }
    /**
     * New access token
     *
     * @param string|null $accessToken
     *
     * @return self
     */
    public function setAccessToken(?string $accessToken): self
    {
        $this->initialized['accessToken'] = true;
        $this->accessToken = $accessToken;
        return $this;
    }
    /**
     * Expiration time in seconds
     *
     * @return int|null
     */
    public function getExpiresIn(): ?int
    {
        return $this->expiresIn;
    }
    /**
     * Expiration time in seconds
     *
     * @param int|null $expiresIn
     *
     * @return self
     */
    public function setExpiresIn(?int $expiresIn): self
    {
        $this->initialized['expiresIn'] = true;
        $this->expiresIn = $expiresIn;
        return $this;
    }
    /**
     * New refresh token (rotation)
     *
     * @return string|null
     */
    public function getRefreshToken(): ?string
    {
        return $this->refreshToken;
    }
    /**
     * New refresh token (rotation)
     *
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
     * Token type (always "Bearer")
     *
     * @return string|null
     */
    public function getTokenType(): ?string
    {
        return $this->tokenType;
    }
    /**
     * Token type (always "Bearer")
     *
     * @param string|null $tokenType
     *
     * @return self
     */
    public function setTokenType(?string $tokenType): self
    {
        $this->initialized['tokenType'] = true;
        $this->tokenType = $tokenType;
        return $this;
    }
}