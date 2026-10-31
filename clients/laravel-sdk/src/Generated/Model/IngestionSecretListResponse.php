<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSecretListResponse
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
     * The source's settings are in a config entry and the secret names it binds are withheld: they are shown only to a caller who can update ingestions (ingestion:update)
     *
     * @var bool|null
     */
    protected $namesHidden;
    /**
     * @var list<IngestionSecretResponse>|null
     */
    protected $secrets;
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
     * The source's settings are in a config entry and the secret names it binds are withheld: they are shown only to a caller who can update ingestions (ingestion:update)
     *
     * @return bool|null
     */
    public function getNamesHidden(): ?bool
    {
        return $this->namesHidden;
    }
    /**
     * The source's settings are in a config entry and the secret names it binds are withheld: they are shown only to a caller who can update ingestions (ingestion:update)
     *
     * @param bool|null $namesHidden
     *
     * @return self
     */
    public function setNamesHidden(?bool $namesHidden): self
    {
        $this->initialized['namesHidden'] = true;
        $this->namesHidden = $namesHidden;
        return $this;
    }
    /**
     * @return list<IngestionSecretResponse>|null
     */
    public function getSecrets(): ?array
    {
        return $this->secrets;
    }
    /**
     * @param list<IngestionSecretResponse>|null $secrets
     *
     * @return self
     */
    public function setSecrets(?array $secrets): self
    {
        $this->initialized['secrets'] = true;
        $this->secrets = $secrets;
        return $this;
    }
}