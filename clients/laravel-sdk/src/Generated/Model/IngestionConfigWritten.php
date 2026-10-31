<?php

namespace FlowCatalyst\Generated\Model;

class IngestionConfigWritten
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
     * The codes of every ingestion of the application and client that names the entry, which the value was checked against
     *
     * @var list<string>|null
     */
    protected $ingestions;
    /**
     * The entry's key; absent while the settings are inline
     *
     * @var string|null
     */
    protected $key;
    /**
     * Why the entry is INVALID
     *
     * @var list<FieldError>|null
     */
    protected $problems;
    /**
     * A value is checked against the kind's schema when written: always for a built-in kind, for an extension's only while a live runtime reports its settings
     *
     * @var bool|null
     */
    protected $schemaChecked;
    /**
     * The secret settings of the entry, for a caller who can update ingestions: the path of each setting, the name of its secret and whether the secret has a value. Never a value. Write a value with secretFields (by field) or secrets (by name)
     *
     * @var list<IngestionConfigSecret>|null
     */
    protected $secrets;
    /**
     * @var string|null
     */
    protected $status;
    /**
     * @var \DateTime|null
     */
    protected $updatedAt;
    /**
     * The entry: the kind's settings as an inline source holds them (an extension kind's settings are the object itself), and optionally targetUrl. Absent when there is no entry, when it is not an object, and when valueHidden
     *
     * @var array<string, mixed>|null
     */
    protected $value;
    /**
     * The entry exists but its value is withheld: it is shown only to a caller who can update ingestions (ingestion:update). The status and problems are shown regardless
     *
     * @var bool|null
     */
    protected $valueHidden;
    /**
     * SCHEMA_UNCHECKED when no live runtime reports the kind's settings, so the value was not checked against them
     *
     * @var list<string>|null
     */
    protected $warnings;
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
     * The codes of every ingestion of the application and client that names the entry, which the value was checked against
     *
     * @return list<string>|null
     */
    public function getIngestions(): ?array
    {
        return $this->ingestions;
    }
    /**
     * The codes of every ingestion of the application and client that names the entry, which the value was checked against
     *
     * @param list<string>|null $ingestions
     *
     * @return self
     */
    public function setIngestions(?array $ingestions): self
    {
        $this->initialized['ingestions'] = true;
        $this->ingestions = $ingestions;
        return $this;
    }
    /**
     * The entry's key; absent while the settings are inline
     *
     * @return string|null
     */
    public function getKey(): ?string
    {
        return $this->key;
    }
    /**
     * The entry's key; absent while the settings are inline
     *
     * @param string|null $key
     *
     * @return self
     */
    public function setKey(?string $key): self
    {
        $this->initialized['key'] = true;
        $this->key = $key;
        return $this;
    }
    /**
     * Why the entry is INVALID
     *
     * @return list<FieldError>|null
     */
    public function getProblems(): ?array
    {
        return $this->problems;
    }
    /**
     * Why the entry is INVALID
     *
     * @param list<FieldError>|null $problems
     *
     * @return self
     */
    public function setProblems(?array $problems): self
    {
        $this->initialized['problems'] = true;
        $this->problems = $problems;
        return $this;
    }
    /**
     * A value is checked against the kind's schema when written: always for a built-in kind, for an extension's only while a live runtime reports its settings
     *
     * @return bool|null
     */
    public function getSchemaChecked(): ?bool
    {
        return $this->schemaChecked;
    }
    /**
     * A value is checked against the kind's schema when written: always for a built-in kind, for an extension's only while a live runtime reports its settings
     *
     * @param bool|null $schemaChecked
     *
     * @return self
     */
    public function setSchemaChecked(?bool $schemaChecked): self
    {
        $this->initialized['schemaChecked'] = true;
        $this->schemaChecked = $schemaChecked;
        return $this;
    }
    /**
     * The secret settings of the entry, for a caller who can update ingestions: the path of each setting, the name of its secret and whether the secret has a value. Never a value. Write a value with secretFields (by field) or secrets (by name)
     *
     * @return list<IngestionConfigSecret>|null
     */
    public function getSecrets(): ?array
    {
        return $this->secrets;
    }
    /**
     * The secret settings of the entry, for a caller who can update ingestions: the path of each setting, the name of its secret and whether the secret has a value. Never a value. Write a value with secretFields (by field) or secrets (by name)
     *
     * @param list<IngestionConfigSecret>|null $secrets
     *
     * @return self
     */
    public function setSecrets(?array $secrets): self
    {
        $this->initialized['secrets'] = true;
        $this->secrets = $secrets;
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
     * The entry: the kind's settings as an inline source holds them (an extension kind's settings are the object itself), and optionally targetUrl. Absent when there is no entry, when it is not an object, and when valueHidden
     *
     * @return array<string, mixed>|null
     */
    public function getValue(): ?iterable
    {
        return $this->value;
    }
    /**
     * The entry: the kind's settings as an inline source holds them (an extension kind's settings are the object itself), and optionally targetUrl. Absent when there is no entry, when it is not an object, and when valueHidden
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
    /**
     * The entry exists but its value is withheld: it is shown only to a caller who can update ingestions (ingestion:update). The status and problems are shown regardless
     *
     * @return bool|null
     */
    public function getValueHidden(): ?bool
    {
        return $this->valueHidden;
    }
    /**
     * The entry exists but its value is withheld: it is shown only to a caller who can update ingestions (ingestion:update). The status and problems are shown regardless
     *
     * @param bool|null $valueHidden
     *
     * @return self
     */
    public function setValueHidden(?bool $valueHidden): self
    {
        $this->initialized['valueHidden'] = true;
        $this->valueHidden = $valueHidden;
        return $this;
    }
    /**
     * SCHEMA_UNCHECKED when no live runtime reports the kind's settings, so the value was not checked against them
     *
     * @return list<string>|null
     */
    public function getWarnings(): ?array
    {
        return $this->warnings;
    }
    /**
     * SCHEMA_UNCHECKED when no live runtime reports the kind's settings, so the value was not checked against them
     *
     * @param list<string>|null $warnings
     *
     * @return self
     */
    public function setWarnings(?array $warnings): self
    {
        $this->initialized['warnings'] = true;
        $this->warnings = $warnings;
        return $this;
    }
}