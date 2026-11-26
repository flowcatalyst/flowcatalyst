<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class ProvisionServiceAccountRequest implements AdditionalPropertiesInterface
{
    use AdditionalAndPatternProperties;
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
     * Replaces the template's path before the credential name; must lie under the platform's publishing prefix.
     *
     * @var string|null
     */
    protected $publishBasePath;
    /**
     * Also publish the provisioned credentials to the configured store (Parameter Store). Omitted takes the platform default; true is refused 400 CREDENTIAL_PUBLISHING_UNAVAILABLE when publishing is not configured.
     *
     * @var bool|null
     */
    protected $publishCredentials;
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
     * Replaces the template's path before the credential name; must lie under the platform's publishing prefix.
     *
     * @return string|null
     */
    public function getPublishBasePath(): ?string
    {
        return $this->publishBasePath;
    }
    /**
     * Replaces the template's path before the credential name; must lie under the platform's publishing prefix.
     *
     * @param string|null $publishBasePath
     *
     * @return self
     */
    public function setPublishBasePath(?string $publishBasePath): self
    {
        $this->initialized['publishBasePath'] = true;
        $this->publishBasePath = $publishBasePath;
        return $this;
    }
    /**
     * Also publish the provisioned credentials to the configured store (Parameter Store). Omitted takes the platform default; true is refused 400 CREDENTIAL_PUBLISHING_UNAVAILABLE when publishing is not configured.
     *
     * @return bool|null
     */
    public function getPublishCredentials(): ?bool
    {
        return $this->publishCredentials;
    }
    /**
     * Also publish the provisioned credentials to the configured store (Parameter Store). Omitted takes the platform default; true is refused 400 CREDENTIAL_PUBLISHING_UNAVAILABLE when publishing is not configured.
     *
     * @param bool|null $publishCredentials
     *
     * @return self
     */
    public function setPublishCredentials(?bool $publishCredentials): self
    {
        $this->initialized['publishCredentials'] = true;
        $this->publishCredentials = $publishCredentials;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'publishBasePath' => ['publishBasePath', 'getPublishBasePath', 'setPublishBasePath'], 'publishCredentials' => ['publishCredentials', 'getPublishCredentials', 'setPublishCredentials']];
    }
}