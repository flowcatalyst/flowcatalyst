<?php

namespace FlowCatalyst\Generated\Model;

class Desired
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
     * @var TokenAuth|null
     */
    protected $auth;
    /**
     * @var list<_Function>|null
     */
    protected $functions;
    /**
     * @var string|null
     */
    protected $pool;
    /**
     * @var int|null
     */
    protected $revision;
    /**
     * @var list<Route>|null
     */
    protected $routes;
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
     * @return TokenAuth|null
     */
    public function getAuth(): ?TokenAuth
    {
        return $this->auth;
    }
    /**
     * @param TokenAuth|null $auth
     *
     * @return self
     */
    public function setAuth(?TokenAuth $auth): self
    {
        $this->initialized['auth'] = true;
        $this->auth = $auth;
        return $this;
    }
    /**
     * @return list<_Function>|null
     */
    public function getFunctions(): ?array
    {
        return $this->functions;
    }
    /**
     * @param list<_Function>|null $functions
     *
     * @return self
     */
    public function setFunctions(?array $functions): self
    {
        $this->initialized['functions'] = true;
        $this->functions = $functions;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getPool(): ?string
    {
        return $this->pool;
    }
    /**
     * @param string|null $pool
     *
     * @return self
     */
    public function setPool(?string $pool): self
    {
        $this->initialized['pool'] = true;
        $this->pool = $pool;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRevision(): ?int
    {
        return $this->revision;
    }
    /**
     * @param int|null $revision
     *
     * @return self
     */
    public function setRevision(?int $revision): self
    {
        $this->initialized['revision'] = true;
        $this->revision = $revision;
        return $this;
    }
    /**
     * @return list<Route>|null
     */
    public function getRoutes(): ?array
    {
        return $this->routes;
    }
    /**
     * @param list<Route>|null $routes
     *
     * @return self
     */
    public function setRoutes(?array $routes): self
    {
        $this->initialized['routes'] = true;
        $this->routes = $routes;
        return $this;
    }
}