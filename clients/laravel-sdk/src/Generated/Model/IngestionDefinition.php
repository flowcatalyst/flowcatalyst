<?php

namespace FlowCatalyst\Generated\Model;

class IngestionDefinition
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
     * Keep the first 2 KB of a payload that failed. Off unless the client has agreed.
     *
     * @var bool|null
     */
    protected $keepFailedPayloads;
    /**
     * @var string|null
     */
    protected $name;
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