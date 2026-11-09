<?php

namespace FlowCatalyst\Generated\Model;

class SyncPushRuleRequest
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
     * {application}:{segment}(:{segment})*, the first segment the application's own code; a segment may be a {placeholder}
     *
     * @var string|null
     */
    protected $pattern;
    /**
     * A deliberately open topic: anyone may subscribe, with or without credentials. Required for a rule with neither {principal} nor requires.
     *
     * @var bool|null
     */
    protected $public;
    /**
     * Permissions the subscriber must all hold
     *
     * @var list<string>|null
     */
    protected $requires;
    /**
     * {application}:{segment}(:{segment})*, the first segment the application's own code; a segment may be a {placeholder}
     *
     * @return string|null
     */
    public function getPattern(): ?string
    {
        return $this->pattern;
    }
    /**
     * {application}:{segment}(:{segment})*, the first segment the application's own code; a segment may be a {placeholder}
     *
     * @param string|null $pattern
     *
     * @return self
     */
    public function setPattern(?string $pattern): self
    {
        $this->initialized['pattern'] = true;
        $this->pattern = $pattern;
        return $this;
    }
    /**
     * A deliberately open topic: anyone may subscribe, with or without credentials. Required for a rule with neither {principal} nor requires.
     *
     * @return bool|null
     */
    public function getPublic(): ?bool
    {
        return $this->public;
    }
    /**
     * A deliberately open topic: anyone may subscribe, with or without credentials. Required for a rule with neither {principal} nor requires.
     *
     * @param bool|null $public
     *
     * @return self
     */
    public function setPublic(?bool $public): self
    {
        $this->initialized['public'] = true;
        $this->public = $public;
        return $this;
    }
    /**
     * Permissions the subscriber must all hold
     *
     * @return list<string>|null
     */
    public function getRequires(): ?array
    {
        return $this->requires;
    }
    /**
     * Permissions the subscriber must all hold
     *
     * @param list<string>|null $requires
     *
     * @return self
     */
    public function setRequires(?array $requires): self
    {
        $this->initialized['requires'] = true;
        $this->requires = $requires;
        return $this;
    }
}