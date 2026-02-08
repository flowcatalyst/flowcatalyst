<?php

namespace FlowCatalyst\Generated\Model;

class CreateServiceAccountResponse extends \ArrayObject
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
     * @var ServiceAccountDto|null
     */
    protected $serviceAccount;
    /**
     * @var string|null
     */
    protected $principalId;
    /**
     * @var OAuthCredentials|null
     */
    protected $oauth;
    /**
     * @var WebhookCredentials|null
     */
    protected $webhook;
    /**
     * @return ServiceAccountDto|null
     */
    public function getServiceAccount(): ?ServiceAccountDto
    {
        return $this->serviceAccount;
    }
    /**
     * @param ServiceAccountDto|null $serviceAccount
     *
     * @return self
     */
    public function setServiceAccount(?ServiceAccountDto $serviceAccount): self
    {
        $this->initialized['serviceAccount'] = true;
        $this->serviceAccount = $serviceAccount;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPrincipalId(): ?string
    {
        return $this->principalId;
    }
    /**
     * @param string|null $principalId
     *
     * @return self
     */
    public function setPrincipalId(?string $principalId): self
    {
        $this->initialized['principalId'] = true;
        $this->principalId = $principalId;
        return $this;
    }
    /**
     * @return OAuthCredentials|null
     */
    public function getOauth(): ?OAuthCredentials
    {
        return $this->oauth;
    }
    /**
     * @param OAuthCredentials|null $oauth
     *
     * @return self
     */
    public function setOauth(?OAuthCredentials $oauth): self
    {
        $this->initialized['oauth'] = true;
        $this->oauth = $oauth;
        return $this;
    }
    /**
     * @return WebhookCredentials|null
     */
    public function getWebhook(): ?WebhookCredentials
    {
        return $this->webhook;
    }
    /**
     * @param WebhookCredentials|null $webhook
     *
     * @return self
     */
    public function setWebhook(?WebhookCredentials $webhook): self
    {
        $this->initialized['webhook'] = true;
        $this->webhook = $webhook;
        return $this;
    }
}