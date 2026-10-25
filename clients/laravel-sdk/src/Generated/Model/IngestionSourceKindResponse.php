<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSourceKindResponse
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
     * @var bool|null
     */
    protected $builtIn;
    /**
     * @var string|null
     */
    protected $description;
    /**
     * @var string|null
     */
    protected $kind;
    /**
     * Creating or changing an ingestion of this kind needs this permission
     *
     * @var string|null
     */
    protected $requiredPermission;
    /**
     * The kind's settings, as the runtime describes them for the form
     *
     * @var mixed|null
     */
    protected $settings;
    /**
     * @var bool|null
     */
    protected $singleOwner;
    /**
     * @return bool|null
     */
    public function getBuiltIn(): ?bool
    {
        return $this->builtIn;
    }
    /**
     * @param bool|null $builtIn
     *
     * @return self
     */
    public function setBuiltIn(?bool $builtIn): self
    {
        $this->initialized['builtIn'] = true;
        $this->builtIn = $builtIn;
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
     * Creating or changing an ingestion of this kind needs this permission
     *
     * @return string|null
     */
    public function getRequiredPermission(): ?string
    {
        return $this->requiredPermission;
    }
    /**
     * Creating or changing an ingestion of this kind needs this permission
     *
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
     * The kind's settings, as the runtime describes them for the form
     *
     * @return mixed
     */
    public function getSettings()
    {
        return $this->settings;
    }
    /**
     * The kind's settings, as the runtime describes them for the form
     *
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
    /**
     * @return bool|null
     */
    public function getSingleOwner(): ?bool
    {
        return $this->singleOwner;
    }
    /**
     * @param bool|null $singleOwner
     *
     * @return self
     */
    public function setSingleOwner(?bool $singleOwner): self
    {
        $this->initialized['singleOwner'] = true;
        $this->singleOwner = $singleOwner;
        return $this;
    }
}