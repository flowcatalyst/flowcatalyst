<?php

namespace FlowCatalyst\Generated\Model;

class TestAnswer extends \ArrayObject
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
     * The start of the answer; the platform keeps it only if the tested definition keeps failed payloads
     *
     * @var string|null
     */
    protected $excerpt;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * @var string|null
     */
    protected $outcome;
    /**
     * @var int|null
     */
    protected $records;
    /**
     * What happened, step by step, for a person; no secret or token
     *
     * @var list<string>|null
     */
    protected $steps;
    /**
     * The start of the answer; the platform keeps it only if the tested definition keeps failed payloads
     *
     * @return string|null
     */
    public function getExcerpt(): ?string
    {
        return $this->excerpt;
    }
    /**
     * The start of the answer; the platform keeps it only if the tested definition keeps failed payloads
     *
     * @param string|null $excerpt
     *
     * @return self
     */
    public function setExcerpt(?string $excerpt): self
    {
        $this->initialized['excerpt'] = true;
        $this->excerpt = $excerpt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getOutcome(): ?string
    {
        return $this->outcome;
    }
    /**
     * @param string|null $outcome
     *
     * @return self
     */
    public function setOutcome(?string $outcome): self
    {
        $this->initialized['outcome'] = true;
        $this->outcome = $outcome;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRecords(): ?int
    {
        return $this->records;
    }
    /**
     * @param int|null $records
     *
     * @return self
     */
    public function setRecords(?int $records): self
    {
        $this->initialized['records'] = true;
        $this->records = $records;
        return $this;
    }
    /**
     * What happened, step by step, for a person; no secret or token
     *
     * @return list<string>|null
     */
    public function getSteps(): ?array
    {
        return $this->steps;
    }
    /**
     * What happened, step by step, for a person; no secret or token
     *
     * @param list<string>|null $steps
     *
     * @return self
     */
    public function setSteps(?array $steps): self
    {
        $this->initialized['steps'] = true;
        $this->steps = $steps;
        return $this;
    }
}