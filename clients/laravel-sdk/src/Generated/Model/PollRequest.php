<?php

namespace FlowCatalyst\Generated\Model;

class PollRequest extends \ArrayObject
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
     * The built-in source kinds the runtime carries (http, sftp, s3, sqs, rabbitmq, database-poll, api-poll); absent is all of them. Only ingestions of a kind the runtime carries are listed to it, and a polled kind's fair share counts only the runtimes that carry it.
     *
     * @var list<string>|null
     */
    protected $builtinKinds;
    /**
     * The contract version the runtime speaks; the platform serves the current one and the one before it
     *
     * @var int|null
     */
    protected $contractVersion;
    /**
     * The runtime process, new on every start (a random UUID). While another instance that polled as this runtimeId is live (polled within one lease and has not said goodbye since), the poll is refused with 409 RUNTIME_ID_IN_USE and changes nothing. Absent: no check.
     *
     * @var string|null
     */
    protected $instanceId;
    /**
     * The kinds the runtime's extensions add (and camel-route)
     *
     * @var list<RuntimeKind>|null
     */
    protected $kinds;
    /**
     * The asking runtime; unique among running runtimes
     *
     * @var string|null
     */
    protected $runtimeId;
    /**
     * Where the runtime can be reached; informational, the platform never calls it
     *
     * @var string|null
     */
    protected $url;
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
     * The built-in source kinds the runtime carries (http, sftp, s3, sqs, rabbitmq, database-poll, api-poll); absent is all of them. Only ingestions of a kind the runtime carries are listed to it, and a polled kind's fair share counts only the runtimes that carry it.
     *
     * @return list<string>|null
     */
    public function getBuiltinKinds(): ?array
    {
        return $this->builtinKinds;
    }
    /**
     * The built-in source kinds the runtime carries (http, sftp, s3, sqs, rabbitmq, database-poll, api-poll); absent is all of them. Only ingestions of a kind the runtime carries are listed to it, and a polled kind's fair share counts only the runtimes that carry it.
     *
     * @param list<string>|null $builtinKinds
     *
     * @return self
     */
    public function setBuiltinKinds(?array $builtinKinds): self
    {
        $this->initialized['builtinKinds'] = true;
        $this->builtinKinds = $builtinKinds;
        return $this;
    }
    /**
     * The contract version the runtime speaks; the platform serves the current one and the one before it
     *
     * @return int|null
     */
    public function getContractVersion(): ?int
    {
        return $this->contractVersion;
    }
    /**
     * The contract version the runtime speaks; the platform serves the current one and the one before it
     *
     * @param int|null $contractVersion
     *
     * @return self
     */
    public function setContractVersion(?int $contractVersion): self
    {
        $this->initialized['contractVersion'] = true;
        $this->contractVersion = $contractVersion;
        return $this;
    }
    /**
     * The runtime process, new on every start (a random UUID). While another instance that polled as this runtimeId is live (polled within one lease and has not said goodbye since), the poll is refused with 409 RUNTIME_ID_IN_USE and changes nothing. Absent: no check.
     *
     * @return string|null
     */
    public function getInstanceId(): ?string
    {
        return $this->instanceId;
    }
    /**
     * The runtime process, new on every start (a random UUID). While another instance that polled as this runtimeId is live (polled within one lease and has not said goodbye since), the poll is refused with 409 RUNTIME_ID_IN_USE and changes nothing. Absent: no check.
     *
     * @param string|null $instanceId
     *
     * @return self
     */
    public function setInstanceId(?string $instanceId): self
    {
        $this->initialized['instanceId'] = true;
        $this->instanceId = $instanceId;
        return $this;
    }
    /**
     * The kinds the runtime's extensions add (and camel-route)
     *
     * @return list<RuntimeKind>|null
     */
    public function getKinds(): ?array
    {
        return $this->kinds;
    }
    /**
     * The kinds the runtime's extensions add (and camel-route)
     *
     * @param list<RuntimeKind>|null $kinds
     *
     * @return self
     */
    public function setKinds(?array $kinds): self
    {
        $this->initialized['kinds'] = true;
        $this->kinds = $kinds;
        return $this;
    }
    /**
     * The asking runtime; unique among running runtimes
     *
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
     * The asking runtime; unique among running runtimes
     *
     * @param string|null $runtimeId
     *
     * @return self
     */
    public function setRuntimeId(?string $runtimeId): self
    {
        $this->initialized['runtimeId'] = true;
        $this->runtimeId = $runtimeId;
        return $this;
    }
    /**
     * Where the runtime can be reached; informational, the platform never calls it
     *
     * @return string|null
     */
    public function getUrl(): ?string
    {
        return $this->url;
    }
    /**
     * Where the runtime can be reached; informational, the platform never calls it
     *
     * @param string|null $url
     *
     * @return self
     */
    public function setUrl(?string $url): self
    {
        $this->initialized['url'] = true;
        $this->url = $url;
        return $this;
    }
}