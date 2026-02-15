<?php

namespace FlowCatalyst\Generated\Model;

class OAuthClientInfo extends \ArrayObject
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
     * OAuth client internal ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * OAuth client_id for authentication
     *
     * @var string|null
     */
    protected $clientId;
    /**
     * OAuth client_secret (only returned at creation time)
     *
     * @var string|null
     */
    protected $clientSecret;
    /**
     * OAuth client internal ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * OAuth client internal ID
     *
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * OAuth client_id for authentication
     *
     * @return string|null
     */
    public function getClientId(): ?string
    {
        return $this->clientId;
    }
    /**
     * OAuth client_id for authentication
     *
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
     * OAuth client_secret (only returned at creation time)
     *
     * @return string|null
     */
    public function getClientSecret(): ?string
    {
        return $this->clientSecret;
    }
    /**
     * OAuth client_secret (only returned at creation time)
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