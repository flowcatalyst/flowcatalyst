<?php

namespace FlowCatalyst\Generated\Model;

class CorsOriginListResponse extends \ArrayObject
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
     * CORS origin entries
     *
     * @var list<CorsOriginDto>|null
     */
    protected $items;
    /**
     * Total count
     *
     * @var int|null
     */
    protected $total;
    /**
     * CORS origin entries
     *
     * @return list<CorsOriginDto>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * CORS origin entries
     *
     * @param list<CorsOriginDto>|null $items
     *
     * @return self
     */
    public function setItems(?array $items): self
    {
        $this->initialized['items'] = true;
        $this->items = $items;
        return $this;
    }
    /**
     * Total count
     *
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * Total count
     *
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}