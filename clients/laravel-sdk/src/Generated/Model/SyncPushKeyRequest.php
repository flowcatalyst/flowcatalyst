<?php

namespace FlowCatalyst\Generated\Model;

class SyncPushKeyRequest
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
    protected $alg;
    /**
     * Ed25519
     *
     * @var string|null
     */
    protected $crv;
    /**
     * The key id tickets name in their header
     *
     * @var string|null
     */
    protected $kid;
    /**
     * OKP
     *
     * @var string|null
     */
    protected $kty;
    /**
     * @var string|null
     */
    protected $use;
    /**
     * The public key, base64url
     *
     * @var string|null
     */
    protected $x;
    /**
     * @return string|null
     */
    public function getAlg(): ?string
    {
        return $this->alg;
    }
    /**
     * @param string|null $alg
     *
     * @return self
     */
    public function setAlg(?string $alg): self
    {
        $this->initialized['alg'] = true;
        $this->alg = $alg;
        return $this;
    }
    /**
     * Ed25519
     *
     * @return string|null
     */
    public function getCrv(): ?string
    {
        return $this->crv;
    }
    /**
     * Ed25519
     *
     * @param string|null $crv
     *
     * @return self
     */
    public function setCrv(?string $crv): self
    {
        $this->initialized['crv'] = true;
        $this->crv = $crv;
        return $this;
    }
    /**
     * The key id tickets name in their header
     *
     * @return string|null
     */
    public function getKid(): ?string
    {
        return $this->kid;
    }
    /**
     * The key id tickets name in their header
     *
     * @param string|null $kid
     *
     * @return self
     */
    public function setKid(?string $kid): self
    {
        $this->initialized['kid'] = true;
        $this->kid = $kid;
        return $this;
    }
    /**
     * OKP
     *
     * @return string|null
     */
    public function getKty(): ?string
    {
        return $this->kty;
    }
    /**
     * OKP
     *
     * @param string|null $kty
     *
     * @return self
     */
    public function setKty(?string $kty): self
    {
        $this->initialized['kty'] = true;
        $this->kty = $kty;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getUse(): ?string
    {
        return $this->use;
    }
    /**
     * @param string|null $use
     *
     * @return self
     */
    public function setUse(?string $use): self
    {
        $this->initialized['use'] = true;
        $this->use = $use;
        return $this;
    }
    /**
     * The public key, base64url
     *
     * @return string|null
     */
    public function getX(): ?string
    {
        return $this->x;
    }
    /**
     * The public key, base64url
     *
     * @param string|null $x
     *
     * @return self
     */
    public function setX(?string $x): self
    {
        $this->initialized['x'] = true;
        $this->x = $x;
        return $this;
    }
}