<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminEventTypesIdGetResponse200 extends \ArrayObject
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
    protected $id;
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
    protected $application;
    /**
     * @var mixed|null
     */
    protected $subdomain;
    /**
     * @var mixed|null
     */
    protected $aggregate;
    /**
     * @var mixed|null
     */
    protected $event;
    /**
     * @var list<ApiAdminEventTypesIdGetResponse200SpecVersionsItem>|null
     */
    protected $specVersions;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var string|null
     */
    protected $source;
    /**
     * @var bool|null
     */
    protected $clientScoped;
    /**
     * @var \DateTime|null
     */
    protected $createdAt;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
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
    public function getApplication()
    {
        return $this->application;
    }
    /**
     * @param mixed $application
     *
     * @return self
     */
    public function setApplication($application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSubdomain()
    {
        return $this->subdomain;
    }
    /**
     * @param mixed $subdomain
     *
     * @return self
     */
    public function setSubdomain($subdomain): self
    {
        $this->initialized['subdomain'] = true;
        $this->subdomain = $subdomain;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getAggregate()
    {
        return $this->aggregate;
    }
    /**
     * @param mixed $aggregate
     *
     * @return self
     */
    public function setAggregate($aggregate): self
    {
        $this->initialized['aggregate'] = true;
        $this->aggregate = $aggregate;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getEvent()
    {
        return $this->event;
    }
    /**
     * @param mixed $event
     *
     * @return self
     */
    public function setEvent($event): self
    {
        $this->initialized['event'] = true;
        $this->event = $event;
        return $this;
    }
    /**
     * @return list<ApiAdminEventTypesIdGetResponse200SpecVersionsItem>|null
     */
    public function getSpecVersions(): ?array
    {
        return $this->specVersions;
    }
    /**
     * @param list<ApiAdminEventTypesIdGetResponse200SpecVersionsItem>|null $specVersions
     *
     * @return self
     */
    public function setSpecVersions(?array $specVersions): self
    {
        $this->initialized['specVersions'] = true;
        $this->specVersions = $specVersions;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getStatus(): ?string
    {
        return $this->status;
    }
    /**
     * @param string|null $status
     *
     * @return self
     */
    public function setStatus(?string $status): self
    {
        $this->initialized['status'] = true;
        $this->status = $status;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSource(): ?string
    {
        return $this->source;
    }
    /**
     * @param string|null $source
     *
     * @return self
     */
    public function setSource(?string $source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * @return bool|null
     */
    public function getClientScoped(): ?bool
    {
        return $this->clientScoped;
    }
    /**
     * @param bool|null $clientScoped
     *
     * @return self
     */
    public function setClientScoped(?bool $clientScoped): self
    {
        $this->initialized['clientScoped'] = true;
        $this->clientScoped = $clientScoped;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getCreatedAt(): ?\DateTime
    {
        return $this->createdAt;
    }
    /**
     * @param \DateTime|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?\DateTime $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
    /**
     * @return \DateTime|null
     */
    public function getUpdatedAt(): ?\DateTime
    {
        return $this->updatedAt;
    }
    /**
     * @param \DateTime|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?\DateTime $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}