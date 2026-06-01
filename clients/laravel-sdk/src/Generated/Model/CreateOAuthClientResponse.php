<?php

namespace FlowCatalyst\Generated\Model;

class CreateOAuthClientResponse extends \ArrayObject
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
     * OAuth client response DTO
     *
     * @var OAuthClientResponse|null
     */
    protected $client;
    /**
     * Plaintext client secret. Only present on creation of CONFIDENTIAL
     * clients. Capture this on the first response — the platform stores
     * only the encrypted form and cannot return it again.
     *
     * @var string|null
     */
    protected $clientSecret;
    /**
     * OAuth client response DTO
     *
     * @return OAuthClientResponse|null
     */
    public function getClient(): ?OAuthClientResponse
    {
        return $this->client;
    }
    /**
     * OAuth client response DTO
     *
     * @param OAuthClientResponse|null $client
     *
     * @return self
     */
    public function setClient(?OAuthClientResponse $client): self
    {
        $this->initialized['client'] = true;
        $this->client = $client;
        return $this;
    }
    /**
     * Plaintext client secret. Only present on creation of CONFIDENTIAL
     * clients. Capture this on the first response — the platform stores
     * only the encrypted form and cannot return it again.
     *
     * @return string|null
     */
    public function getClientSecret(): ?string
    {
        return $this->clientSecret;
    }
    /**
    * Plaintext client secret. Only present on creation of CONFIDENTIAL
    clients. Capture this on the first response — the platform stores
    only the encrypted form and cannot return it again.
    *
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
}