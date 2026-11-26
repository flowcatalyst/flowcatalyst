<?php

namespace FlowCatalyst\Generated\Model;

use FlowCatalyst\Generated\Runtime\AdditionalAndPatternProperties;
use FlowCatalyst\Generated\Runtime\AdditionalPropertiesInterface;
class EnableRequest implements AdditionalPropertiesInterface
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
     * Replaces the template's path before the credential name; must start and end with '/' and lie under the deployment's prefix. Omitted takes the template.
     *
     * @var string|null
     */
    protected $basePath;
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
     * Replaces the template's path before the credential name; must start and end with '/' and lie under the deployment's prefix. Omitted takes the template.
     *
     * @return string|null
     */
    public function getBasePath(): ?string
    {
        return $this->basePath;
    }
    /**
     * Replaces the template's path before the credential name; must start and end with '/' and lie under the deployment's prefix. Omitted takes the template.
     *
     * @param string|null $basePath
     *
     * @return self
     */
    public function setBasePath(?string $basePath): self
    {
        $this->initialized['basePath'] = true;
        $this->basePath = $basePath;
        return $this;
    }
    public function definedProperties(): array
    {
        return ['dollarSchema' => ['$schema', 'getDollarSchema', 'setDollarSchema'], 'basePath' => ['basePath', 'getBasePath', 'setBasePath']];
    }
}