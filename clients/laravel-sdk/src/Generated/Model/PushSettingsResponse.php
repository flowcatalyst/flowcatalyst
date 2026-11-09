<?php

namespace FlowCatalyst\Generated\Model;

class PushSettingsResponse
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
     * @var string|null
     */
    protected $applicationCode;
    /**
     * @var PushSettingsResponseEffectiveStruct|null
     */
    protected $effective;
    /**
     * @var PushSettings|null
     */
    protected $settings;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * @var string|null
     */
    protected $updatedBy;
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
     * @return string|null
     */
    public function getApplicationCode(): ?string
    {
        return $this->applicationCode;
    }
    /**
     * @param string|null $applicationCode
     *
     * @return self
     */
    public function setApplicationCode(?string $applicationCode): self
    {
        $this->initialized['applicationCode'] = true;
        $this->applicationCode = $applicationCode;
        return $this;
    }
    /**
     * @return PushSettingsResponseEffectiveStruct|null
     */
    public function getEffective(): ?PushSettingsResponseEffectiveStruct
    {
        return $this->effective;
    }
    /**
     * @param PushSettingsResponseEffectiveStruct|null $effective
     *
     * @return self
     */
    public function setEffective(?PushSettingsResponseEffectiveStruct $effective): self
    {
        $this->initialized['effective'] = true;
        $this->effective = $effective;
        return $this;
    }
    /**
     * @return PushSettings|null
     */
    public function getSettings(): ?PushSettings
    {
        return $this->settings;
    }
    /**
     * @param PushSettings|null $settings
     *
     * @return self
     */
    public function setSettings(?PushSettings $settings): self
    {
        $this->initialized['settings'] = true;
        $this->settings = $settings;
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
    /**
     * @return string|null
     */
    public function getUpdatedBy(): ?string
    {
        return $this->updatedBy;
    }
    /**
     * @param string|null $updatedBy
     *
     * @return self
     */
    public function setUpdatedBy(?string $updatedBy): self
    {
        $this->initialized['updatedBy'] = true;
        $this->updatedBy = $updatedBy;
        return $this;
    }
}