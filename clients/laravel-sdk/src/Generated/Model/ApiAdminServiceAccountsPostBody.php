<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminServiceAccountsPostBody extends \ArrayObject
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
    protected $code;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var mixed|null
     */
    protected $description;
    /**
     * @var mixed|null
     */
    protected $applicationId;
    /**
     * @var list<string>|null
     */
    protected $clientIds;
    /**
     * @var mixed|null
     */
    protected $scope;
    /**
     * @var mixed|null
     */
    protected $webhookAuthType;
    /**
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
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
     * @return mixed
     */
    public function getDescription()
    {
        return $this->description;
    }
    /**
     * @param mixed $description
     *
     * @return self
     */
    public function setDescription($description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getApplicationId()
    {
        return $this->applicationId;
    }
    /**
     * @param mixed $applicationId
     *
     * @return self
     */
    public function setApplicationId($applicationId): self
    {
        $this->initialized['applicationId'] = true;
        $this->applicationId = $applicationId;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getClientIds(): ?array
    {
        return $this->clientIds;
    }
    /**
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
     * @return mixed
     */
    public function getScope()
    {
        return $this->scope;
    }
    /**
     * @param mixed $scope
     *
     * @return self
     */
    public function setScope($scope): self
    {
        $this->initialized['scope'] = true;
        $this->scope = $scope;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getWebhookAuthType()
    {
        return $this->webhookAuthType;
    }
    /**
     * @param mixed $webhookAuthType
     *
     * @return self
     */
    public function setWebhookAuthType($webhookAuthType): self
    {
        $this->initialized['webhookAuthType'] = true;
        $this->webhookAuthType = $webhookAuthType;
        return $this;
    }
}