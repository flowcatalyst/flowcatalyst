<?php

namespace FlowCatalyst\Generated\Model;

class TokenAuth
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
    protected $audience;
    /**
     * @var string|null
     */
    protected $issuer;
    /**
     * @return string|null
     */
    public function getAudience(): ?string
    {
        return $this->audience;
    }
    /**
     * @param string|null $audience
     *
     * @return self
     */
    public function setAudience(?string $audience): self
    {
        $this->initialized['audience'] = true;
        $this->audience = $audience;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getIssuer(): ?string
    {
        return $this->issuer;
    }
    /**
     * @param string|null $issuer
     *
     * @return self
     */
    public function setIssuer(?string $issuer): self
    {
        $this->initialized['issuer'] = true;
        $this->issuer = $issuer;
        return $this;
    }
}