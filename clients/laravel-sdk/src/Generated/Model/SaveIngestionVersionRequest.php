<?php

namespace FlowCatalyst\Generated\Model;

class SaveIngestionVersionRequest extends \ArrayObject
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
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @var bool|null
     */
    protected $keepFailedPayloads;
    /**
     * Also make the saved version live (needs the promote permission). Default false.
     *
     * @var bool|null
     */
    protected $live;
    /**
     * @var string|null
     */
    protected $name;
    /**
     * @var string|null
     */
    protected $note;
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @var mixed|null
     */
    protected $source;
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @var mixed|null
     */
    protected $split;
    /**
     * @var Target|null
     */
    protected $target;
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
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @return bool|null
     */
    public function getKeepFailedPayloads(): ?bool
    {
        return $this->keepFailedPayloads;
    }
    /**
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @param bool|null $keepFailedPayloads
     *
     * @return self
     */
    public function setKeepFailedPayloads(?bool $keepFailedPayloads): self
    {
        $this->initialized['keepFailedPayloads'] = true;
        $this->keepFailedPayloads = $keepFailedPayloads;
        return $this;
    }
    /**
     * Also make the saved version live (needs the promote permission). Default false.
     *
     * @return bool|null
     */
    public function getLive(): ?bool
    {
        return $this->live;
    }
    /**
     * Also make the saved version live (needs the promote permission). Default false.
     *
     * @param bool|null $live
     *
     * @return self
     */
    public function setLive(?bool $live): self
    {
        $this->initialized['live'] = true;
        $this->live = $live;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getNote(): ?string
    {
        return $this->note;
    }
    /**
     * @param string|null $note
     *
     * @return self
     */
    public function setNote(?string $note): self
    {
        $this->initialized['note'] = true;
        $this->note = $note;
        return $this;
    }
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @return mixed
     */
    public function getSource()
    {
        return $this->source;
    }
    /**
     * Where the data comes from: {kind: http|sftp|s3|sqs|rabbitmq|database-poll|api-poll|custom, ...}
     *
     * @param mixed $source
     *
     * @return self
     */
    public function setSource($source): self
    {
        $this->initialized['source'] = true;
        $this->source = $source;
        return $this;
    }
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @return mixed
     */
    public function getSplit()
    {
        return $this->split;
    }
    /**
     * How a payload becomes records: {kind: none|csv|json|xml, ...}
     *
     * @param mixed $split
     *
     * @return self
     */
    public function setSplit($split): self
    {
        $this->initialized['split'] = true;
        $this->split = $split;
        return $this;
    }
    /**
     * @return Target|null
     */
    public function getTarget(): ?Target
    {
        return $this->target;
    }
    /**
     * @param Target|null $target
     *
     * @return self
     */
    public function setTarget(?Target $target): self
    {
        $this->initialized['target'] = true;
        $this->target = $target;
        return $this;
    }
}