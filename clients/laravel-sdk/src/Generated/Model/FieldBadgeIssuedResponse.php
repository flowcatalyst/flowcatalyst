<?php

namespace FlowCatalyst\Generated\Model;

class FieldBadgeIssuedResponse
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
     * @var FieldBadgeResponse|null
     */
    protected $badge;
    /**
     * @var string|null
     */
    protected $token;
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
     * @return FieldBadgeResponse|null
     */
    public function getBadge(): ?FieldBadgeResponse
    {
        return $this->badge;
    }
    /**
     * @param FieldBadgeResponse|null $badge
     *
     * @return self
     */
    public function setBadge(?FieldBadgeResponse $badge): self
    {
        $this->initialized['badge'] = true;
        $this->badge = $badge;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getToken(): ?string
    {
        return $this->token;
    }
    /**
     * @param string|null $token
     *
     * @return self
     */
    public function setToken(?string $token): self
    {
        $this->initialized['token'] = true;
        $this->token = $token;
        return $this;
    }
}