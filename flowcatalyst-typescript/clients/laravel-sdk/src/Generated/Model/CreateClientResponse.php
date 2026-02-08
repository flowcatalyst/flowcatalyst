<?php

namespace FlowCatalyst\Generated\Model;

class CreateClientResponse extends \ArrayObject
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
     * @var ClientDto2|null
     */
    protected $client;
    /**
     * @var string|null
     */
    protected $clientSecret;
    /**
     * @return ClientDto2|null
     */
    public function getClient(): ?ClientDto2
    {
        return $this->client;
    }
    /**
     * @param ClientDto2|null $client
     *
     * @return self
     */
    public function setClient(?ClientDto2 $client): self
    {
        $this->initialized['client'] = true;
        $this->client = $client;
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