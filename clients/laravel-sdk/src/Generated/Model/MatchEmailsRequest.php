<?php

namespace FlowCatalyst\Generated\Model;

class MatchEmailsRequest extends \ArrayObject
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
     * Addresses (each entry is parsed like text)
     *
     * @var list<string>|null
     */
    protected $emails;
    /**
     * Pasted address list: commas, semicolons, whitespace or 'Name <addr>' entries
     *
     * @var string|null
     */
    protected $text;
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
     * Addresses (each entry is parsed like text)
     *
     * @return list<string>|null
     */
    public function getEmails(): ?array
    {
        return $this->emails;
    }
    /**
     * Addresses (each entry is parsed like text)
     *
     * @param list<string>|null $emails
     *
     * @return self
     */
    public function setEmails(?array $emails): self
    {
        $this->initialized['emails'] = true;
        $this->emails = $emails;
        return $this;
    }
    /**
     * Pasted address list: commas, semicolons, whitespace or 'Name <addr>' entries
     *
     * @return string|null
     */
    public function getText(): ?string
    {
        return $this->text;
    }
    /**
     * Pasted address list: commas, semicolons, whitespace or 'Name <addr>' entries
     *
     * @param string|null $text
     *
     * @return self
     */
    public function setText(?string $text): self
    {
        $this->initialized['text'] = true;
        $this->text = $text;
        return $this;
    }
}