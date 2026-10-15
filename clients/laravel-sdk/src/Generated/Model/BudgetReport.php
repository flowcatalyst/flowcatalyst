<?php

namespace FlowCatalyst\Generated\Model;

class BudgetReport
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
     * @var int|null
     */
    protected $budgetBytes;
    /**
     * @var int|null
     */
    protected $compiledBytes;
    /**
     * @var int|null
     */
    protected $evictions;
    /**
     * @var int|null
     */
    protected $limitBytes;
    /**
     * @var int|null
     */
    protected $linearBytes;
    /**
     * @var int|null
     */
    protected $loaded;
    /**
     * @var int|null
     */
    protected $refused;
    /**
     * @var int|null
     */
    protected $reserveBytes;
    /**
     * @var int|null
     */
    protected $rssBytes;
    /**
     * @return int|null
     */
    public function getBudgetBytes(): ?int
    {
        return $this->budgetBytes;
    }
    /**
     * @param int|null $budgetBytes
     *
     * @return self
     */
    public function setBudgetBytes(?int $budgetBytes): self
    {
        $this->initialized['budgetBytes'] = true;
        $this->budgetBytes = $budgetBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getCompiledBytes(): ?int
    {
        return $this->compiledBytes;
    }
    /**
     * @param int|null $compiledBytes
     *
     * @return self
     */
    public function setCompiledBytes(?int $compiledBytes): self
    {
        $this->initialized['compiledBytes'] = true;
        $this->compiledBytes = $compiledBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getEvictions(): ?int
    {
        return $this->evictions;
    }
    /**
     * @param int|null $evictions
     *
     * @return self
     */
    public function setEvictions(?int $evictions): self
    {
        $this->initialized['evictions'] = true;
        $this->evictions = $evictions;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getLimitBytes(): ?int
    {
        return $this->limitBytes;
    }
    /**
     * @param int|null $limitBytes
     *
     * @return self
     */
    public function setLimitBytes(?int $limitBytes): self
    {
        $this->initialized['limitBytes'] = true;
        $this->limitBytes = $limitBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getLinearBytes(): ?int
    {
        return $this->linearBytes;
    }
    /**
     * @param int|null $linearBytes
     *
     * @return self
     */
    public function setLinearBytes(?int $linearBytes): self
    {
        $this->initialized['linearBytes'] = true;
        $this->linearBytes = $linearBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getLoaded(): ?int
    {
        return $this->loaded;
    }
    /**
     * @param int|null $loaded
     *
     * @return self
     */
    public function setLoaded(?int $loaded): self
    {
        $this->initialized['loaded'] = true;
        $this->loaded = $loaded;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRefused(): ?int
    {
        return $this->refused;
    }
    /**
     * @param int|null $refused
     *
     * @return self
     */
    public function setRefused(?int $refused): self
    {
        $this->initialized['refused'] = true;
        $this->refused = $refused;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getReserveBytes(): ?int
    {
        return $this->reserveBytes;
    }
    /**
     * @param int|null $reserveBytes
     *
     * @return self
     */
    public function setReserveBytes(?int $reserveBytes): self
    {
        $this->initialized['reserveBytes'] = true;
        $this->reserveBytes = $reserveBytes;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getRssBytes(): ?int
    {
        return $this->rssBytes;
    }
    /**
     * @param int|null $rssBytes
     *
     * @return self
     */
    public function setRssBytes(?int $rssBytes): self
    {
        $this->initialized['rssBytes'] = true;
        $this->rssBytes = $rssBytes;
        return $this;
    }
}