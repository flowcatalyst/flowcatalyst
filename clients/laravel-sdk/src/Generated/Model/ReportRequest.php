<?php

namespace FlowCatalyst\Generated\Model;

class ReportRequest extends \ArrayObject
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
     * @var list<Failure>|null
     */
    protected $failures;
    /**
     * @var list<ExchangeWindow>|null
     */
    protected $metrics;
    /**
     * @var list<RouteState>|null
     */
    protected $routes;
    /**
     * @var string|null
     */
    protected $runtimeId;
    /**
     * @var list<TestAnswer>|null
     */
    protected $tests;
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
     * @return list<Failure>|null
     */
    public function getFailures(): ?array
    {
        return $this->failures;
    }
    /**
     * @param list<Failure>|null $failures
     *
     * @return self
     */
    public function setFailures(?array $failures): self
    {
        $this->initialized['failures'] = true;
        $this->failures = $failures;
        return $this;
    }
    /**
     * @return list<ExchangeWindow>|null
     */
    public function getMetrics(): ?array
    {
        return $this->metrics;
    }
    /**
     * @param list<ExchangeWindow>|null $metrics
     *
     * @return self
     */
    public function setMetrics(?array $metrics): self
    {
        $this->initialized['metrics'] = true;
        $this->metrics = $metrics;
        return $this;
    }
    /**
     * @return list<RouteState>|null
     */
    public function getRoutes(): ?array
    {
        return $this->routes;
    }
    /**
     * @param list<RouteState>|null $routes
     *
     * @return self
     */
    public function setRoutes(?array $routes): self
    {
        $this->initialized['routes'] = true;
        $this->routes = $routes;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getRuntimeId(): ?string
    {
        return $this->runtimeId;
    }
    /**
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
     * @return list<TestAnswer>|null
     */
    public function getTests(): ?array
    {
        return $this->tests;
    }
    /**
     * @param list<TestAnswer>|null $tests
     *
     * @return self
     */
    public function setTests(?array $tests): self
    {
        $this->initialized['tests'] = true;
        $this->tests = $tests;
        return $this;
    }
}