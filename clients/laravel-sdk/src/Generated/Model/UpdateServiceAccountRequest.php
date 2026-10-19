<?php

namespace FlowCatalyst\Generated\Model;

class UpdateServiceAccountRequest extends \ArrayObject
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
     * Client links; replaces the set when present (an empty array clears it). Their count decides the token tier.
     *
     * @var list<string>|null
     */
    protected $clientIds;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * Descriptive tier: ANCHOR (no clientIds), CLIENT (one) or PARTNER (several). Tokens follow the client links, so a scope that disagrees with them is refused 400 SCOPE_REACH_MISMATCH; change both together, or omit it.
     *
     * @var string|null
     */
    protected $scope;
    /**
     * @var WebhookCredentialsDTO|null
     */
    protected $webhookCredentials;
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
     * Client links; replaces the set when present (an empty array clears it). Their count decides the token tier.
     *
     * @return list<string>|null
     */
    public function getClientIds(): ?array
    {
        return $this->clientIds;
    }
    /**
     * Client links; replaces the set when present (an empty array clears it). Their count decides the token tier.
     *
     * @param list<string>|null $clientIds
     *
     * @return self
     */
    public function setClientIds(?array $clientIds): self
    {
        $this->initialized['clientIds'] = true;
        $this->clientIds = $clientIds;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * Descriptive tier: ANCHOR (no clientIds), CLIENT (one) or PARTNER (several). Tokens follow the client links, so a scope that disagrees with them is refused 400 SCOPE_REACH_MISMATCH; change both together, or omit it.
     *
     * @return string|null
     */
    public function getScope(): ?string
    {
        return $this->scope;
    }
    /**
     * Descriptive tier: ANCHOR (no clientIds), CLIENT (one) or PARTNER (several). Tokens follow the client links, so a scope that disagrees with them is refused 400 SCOPE_REACH_MISMATCH; change both together, or omit it.
     *
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
    /**
     * @return WebhookCredentialsDTO|null
     */
    public function getWebhookCredentials(): ?WebhookCredentialsDTO
    {
        return $this->webhookCredentials;
    }
    /**
     * @param WebhookCredentialsDTO|null $webhookCredentials
     *
     * @return self
     */
    public function setWebhookCredentials(?WebhookCredentialsDTO $webhookCredentials): self
    {
        $this->initialized['webhookCredentials'] = true;
        $this->webhookCredentials = $webhookCredentials;
        return $this;
    }
}