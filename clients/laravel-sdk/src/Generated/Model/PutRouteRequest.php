<?php

namespace FlowCatalyst\Generated\Model;

class PutRouteRequest extends \ArrayObject
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
     * An existing alias name to route to; omitted routes to live
     *
     * @var string|null
     */
    protected $alias;
    /**
     * Opt-in alias prefixes: for each prefix p, "p-<hostname>" serves alias p (live routes only; no hyphens)
     *
     * @var list<string>|null
     */
    protected $aliasPrefixes;
    /**
     * A lowercase DNS hostname covered by a claimed zone (no port, no wildcard)
     *
     * @var string|null
     */
    protected $hostname;
    /**
     * The path prefix routed to this function, e.g. "/" or "/webhooks"
     *
     * @var string|null
     */
    protected $pathPrefix;
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
     * An existing alias name to route to; omitted routes to live
     *
     * @return string|null
     */
    public function getAlias(): ?string
    {
        return $this->alias;
    }
    /**
     * An existing alias name to route to; omitted routes to live
     *
     * @param string|null $alias
     *
     * @return self
     */
    public function setAlias(?string $alias): self
    {
        $this->initialized['alias'] = true;
        $this->alias = $alias;
        return $this;
    }
    /**
     * Opt-in alias prefixes: for each prefix p, "p-<hostname>" serves alias p (live routes only; no hyphens)
     *
     * @return list<string>|null
     */
    public function getAliasPrefixes(): ?array
    {
        return $this->aliasPrefixes;
    }
    /**
     * Opt-in alias prefixes: for each prefix p, "p-<hostname>" serves alias p (live routes only; no hyphens)
     *
     * @param list<string>|null $aliasPrefixes
     *
     * @return self
     */
    public function setAliasPrefixes(?array $aliasPrefixes): self
    {
        $this->initialized['aliasPrefixes'] = true;
        $this->aliasPrefixes = $aliasPrefixes;
        return $this;
    }
    /**
     * A lowercase DNS hostname covered by a claimed zone (no port, no wildcard)
     *
     * @return string|null
     */
    public function getHostname(): ?string
    {
        return $this->hostname;
    }
    /**
     * A lowercase DNS hostname covered by a claimed zone (no port, no wildcard)
     *
     * @param string|null $hostname
     *
     * @return self
     */
    public function setHostname(?string $hostname): self
    {
        $this->initialized['hostname'] = true;
        $this->hostname = $hostname;
        return $this;
    }
    /**
     * The path prefix routed to this function, e.g. "/" or "/webhooks"
     *
     * @return string|null
     */
    public function getPathPrefix(): ?string
    {
        return $this->pathPrefix;
    }
    /**
     * The path prefix routed to this function, e.g. "/" or "/webhooks"
     *
     * @param string|null $pathPrefix
     *
     * @return self
     */
    public function setPathPrefix(?string $pathPrefix): self
    {
        $this->initialized['pathPrefix'] = true;
        $this->pathPrefix = $pathPrefix;
        return $this;
    }
}