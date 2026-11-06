<?php

namespace FlowCatalyst\Generated\Model;

class MatchEmailsResponse
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
     * @var list<string>|null
     */
    protected $invalid;
    /**
     * @var list<PrincipalResponse>|null
     */
    protected $matches;
    /**
     * @var list<string>|null
     */
    protected $unmatched;
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
     * @return list<string>|null
     */
    public function getInvalid(): ?array
    {
        return $this->invalid;
    }
    /**
     * @param list<string>|null $invalid
     *
     * @return self
     */
    public function setInvalid(?array $invalid): self
    {
        $this->initialized['invalid'] = true;
        $this->invalid = $invalid;
        return $this;
    }
    /**
     * @return list<PrincipalResponse>|null
     */
    public function getMatches(): ?array
    {
        return $this->matches;
    }
    /**
     * @param list<PrincipalResponse>|null $matches
     *
     * @return self
     */
    public function setMatches(?array $matches): self
    {
        $this->initialized['matches'] = true;
        $this->matches = $matches;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getUnmatched(): ?array
    {
        return $this->unmatched;
    }
    /**
     * @param list<string>|null $unmatched
     *
     * @return self
     */
    public function setUnmatched(?array $unmatched): self
    {
        $this->initialized['unmatched'] = true;
        $this->unmatched = $unmatched;
        return $this;
    }
}