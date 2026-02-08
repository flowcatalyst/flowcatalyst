<?php

namespace FlowCatalyst\Generated\Model;

class CreateEventTypeRequest1 extends \ArrayObject
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
    protected $application;
    /**
     * @var string|null
     */
    protected $subdomain;
    /**
     * @var string|null
     */
    protected $aggregate;
    /**
     * @var string|null
     */
    protected $event;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var bool|null
     */
    protected $clientScoped;
    /**
     * @return string|null
     */
    public function getApplication(): ?string
    {
        return $this->application;
    }
    /**
     * @param string|null $application
     *
     * @return self
     */
    public function setApplication(?string $application): self
    {
        $this->initialized['application'] = true;
        $this->application = $application;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getSubdomain(): ?string
    {
        return $this->subdomain;
    }
    /**
     * @param string|null $subdomain
     *
     * @return self
     */
    public function setSubdomain(?string $subdomain): self
    {
        $this->initialized['subdomain'] = true;
        $this->subdomain = $subdomain;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAggregate(): ?string
    {
        return $this->aggregate;
    }
    /**
     * @param string|null $aggregate
     *
     * @return self
     */
    public function setAggregate(?string $aggregate): self
    {
        $this->initialized['aggregate'] = true;
        $this->aggregate = $aggregate;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getEvent(): ?string
    {
        return $this->event;
    }
    /**
     * @param string|null $event
     *
     * @return self
     */
    public function setEvent(?string $event): self
    {
        $this->initialized['event'] = true;
        $this->event = $event;
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
}