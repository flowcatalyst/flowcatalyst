<?php

namespace FlowCatalyst\Generated\Model;

class SetIngestionConfigRequest extends \ArrayObject
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
     * Secret values by the path of the setting they belong to (password, credential.secretAccessKey, auth.clientSecret). The platform names the secret: the name the value gives the setting, else the one the entry has now, else the entry's key and the path in kebab case (mix-positions-client-secret), writes the name into the value and the secret under it
     *
     * @var array<string, string>|null
     */
    protected $secretFields;
    /**
     * Secret values by secret name, written with the entry as SECRET values of the application's secrets section at the ingestion's client. Each name must be one the value names. Needs no bind-secrets permission for a name the write gives a value
     *
     * @var array<string, string>|null
     */
    protected $secrets;
    /**
     * The entry: the kind's settings as an inline source holds them, and optionally targetUrl. A secret setting holds the secret's name
     *
     * @var array<string, mixed>|null
     */
    protected $value;
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
     * Secret values by the path of the setting they belong to (password, credential.secretAccessKey, auth.clientSecret). The platform names the secret: the name the value gives the setting, else the one the entry has now, else the entry's key and the path in kebab case (mix-positions-client-secret), writes the name into the value and the secret under it
     *
     * @return array<string, string>|null
     */
    public function getSecretFields(): ?iterable
    {
        return $this->secretFields;
    }
    /**
     * Secret values by the path of the setting they belong to (password, credential.secretAccessKey, auth.clientSecret). The platform names the secret: the name the value gives the setting, else the one the entry has now, else the entry's key and the path in kebab case (mix-positions-client-secret), writes the name into the value and the secret under it
     *
     * @param array<string, string>|null $secretFields
     *
     * @return self
     */
    public function setSecretFields(?iterable $secretFields): self
    {
        $this->initialized['secretFields'] = true;
        $this->secretFields = $secretFields;
        return $this;
    }
    /**
     * Secret values by secret name, written with the entry as SECRET values of the application's secrets section at the ingestion's client. Each name must be one the value names. Needs no bind-secrets permission for a name the write gives a value
     *
     * @return array<string, string>|null
     */
    public function getSecrets(): ?iterable
    {
        return $this->secrets;
    }
    /**
     * Secret values by secret name, written with the entry as SECRET values of the application's secrets section at the ingestion's client. Each name must be one the value names. Needs no bind-secrets permission for a name the write gives a value
     *
     * @param array<string, string>|null $secrets
     *
     * @return self
     */
    public function setSecrets(?iterable $secrets): self
    {
        $this->initialized['secrets'] = true;
        $this->secrets = $secrets;
        return $this;
    }
    /**
     * The entry: the kind's settings as an inline source holds them, and optionally targetUrl. A secret setting holds the secret's name
     *
     * @return array<string, mixed>|null
     */
    public function getValue(): ?iterable
    {
        return $this->value;
    }
    /**
     * The entry: the kind's settings as an inline source holds them, and optionally targetUrl. A secret setting holds the secret's name
     *
     * @param array<string, mixed>|null $value
     *
     * @return self
     */
    public function setValue(?iterable $value): self
    {
        $this->initialized['value'] = true;
        $this->value = $value;
        return $this;
    }
}