<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsPostResponse201 extends \ArrayObject
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
     * @var ApiAdminServiceAccountsPostResponse201ServiceAccount|null
     */
    protected $serviceAccount;
    /**
     * @var string|null
     */
    protected $principalId;
    /**
     * @var ApiAdminServiceAccountsPostResponse201Oauth|null
     */
    protected $oauth;
    /**
     * @var ApiAdminServiceAccountsPostResponse201Webhook|null
     */
    protected $webhook;
    /**
     * @return ApiAdminServiceAccountsPostResponse201ServiceAccount|null
     */
    public function getServiceAccount(): ?ApiAdminServiceAccountsPostResponse201ServiceAccount
    {
        return $this->serviceAccount;
    }
    /**
     * @param ApiAdminServiceAccountsPostResponse201ServiceAccount|null $serviceAccount
     *
     * @return self
     */
    public function setServiceAccount(?ApiAdminServiceAccountsPostResponse201ServiceAccount $serviceAccount): self
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
     * @return ApiAdminServiceAccountsPostResponse201Oauth|null
     */
    public function getOauth(): ?ApiAdminServiceAccountsPostResponse201Oauth
    {
        return $this->oauth;
    }
    /**
     * @param ApiAdminServiceAccountsPostResponse201Oauth|null $oauth
     *
     * @return self
     */
    public function setOauth(?ApiAdminServiceAccountsPostResponse201Oauth $oauth): self
    {
        $this->initialized['oauth'] = true;
        $this->oauth = $oauth;
        return $this;
    }
    /**
     * @return ApiAdminServiceAccountsPostResponse201Webhook|null
     */
    public function getWebhook(): ?ApiAdminServiceAccountsPostResponse201Webhook
    {
        return $this->webhook;
    }
    /**
     * @param ApiAdminServiceAccountsPostResponse201Webhook|null $webhook
     *
     * @return self
     */
    public function setWebhook(?ApiAdminServiceAccountsPostResponse201Webhook $webhook): self
    {
        $this->initialized['webhook'] = true;
        $this->webhook = $webhook;
        return $this;
    }
}