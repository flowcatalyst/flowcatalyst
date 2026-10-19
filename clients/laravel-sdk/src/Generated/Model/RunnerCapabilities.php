<?php

namespace FlowCatalyst\Generated\Model;

class RunnerCapabilities
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
     * @var array<string, mixed>|null
     */
    protected $extra;
    /**
     * @var int|null
     */
    protected $protocol;
    /**
     * @var string|null
     */
    protected $runnerKind;
    /**
     * @var list<string>|null
     */
    protected $runtimes;
    /**
     * @var string|null
     */
    protected $version;
    /**
     * @return array<string, mixed>|null
     */
    public function getExtra(): ?iterable
    {
        return $this->extra;
    }
    /**
     * @param array<string, mixed>|null $extra
     *
     * @return self
     */
    public function setExtra(?iterable $extra): self
    {
        $this->initialized['extra'] = true;
        $this->extra = $extra;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getProtocol(): ?int
    {
        return $this->protocol;
    }
    /**
     * @param int|null $protocol
     *
     * @return self
     */
    public function setProtocol(?int $protocol): self
    {
        $this->initialized['protocol'] = true;
        $this->protocol = $protocol;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRunnerKind(): ?string
    {
        return $this->runnerKind;
    }
    /**
     * @param string|null $runnerKind
     *
     * @return self
     */
    public function setRunnerKind(?string $runnerKind): self
    {
        $this->initialized['runnerKind'] = true;
        $this->runnerKind = $runnerKind;
        return $this;
    }
    /**
     * @return list<string>|null
     */
    public function getRuntimes(): ?array
    {
        return $this->runtimes;
    }
    /**
     * @param list<string>|null $runtimes
     *
     * @return self
     */
    public function setRuntimes(?array $runtimes): self
    {
        $this->initialized['runtimes'] = true;
        $this->runtimes = $runtimes;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getVersion(): ?string
    {
        return $this->version;
    }
    /**
     * @param string|null $version
     *
     * @return self
     */
    public function setVersion(?string $version): self
    {
        $this->initialized['version'] = true;
        $this->version = $version;
        return $this;
    }
}