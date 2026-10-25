<?php

namespace FlowCatalyst\Generated\Model;

class RuntimeKind extends \ArrayObject
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
    protected $description;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * @var string|null
     */
    protected $requiredPermission;
    /**
     * @var mixed|null
     */
    protected $settings;
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
    public function getKind(): ?string
    {
        return $this->kind;
    }
    /**
     * @param string|null $kind
     *
     * @return self
     */
    public function setKind(?string $kind): self
    {
        $this->initialized['kind'] = true;
        $this->kind = $kind;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRequiredPermission(): ?string
    {
        return $this->requiredPermission;
    }
    /**
     * @param string|null $requiredPermission
     *
     * @return self
     */
    public function setRequiredPermission(?string $requiredPermission): self
    {
        $this->initialized['requiredPermission'] = true;
        $this->requiredPermission = $requiredPermission;
        return $this;
    }
    /**
     * @return mixed
     */
    public function getSettings()
    {
        return $this->settings;
    }
    /**
     * @param mixed $settings
     *
     * @return self
     */
    public function setSettings($settings): self
    {
        $this->initialized['settings'] = true;
        $this->settings = $settings;
        return $this;
    }
}