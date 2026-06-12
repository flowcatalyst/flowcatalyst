<?php

namespace FlowCatalyst\Generated\Model;

class RotateOAuthClientSecretResponse
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
     * @var string|null
     */
    protected $clientId;
    /**
     * @var string|null
     */
    protected $clientSecret;
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
}