<?php

namespace FlowCatalyst\Generated\Model;

class BffEventsGetResponse200 extends \ArrayObject
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
     * @var list<BffEventsGetResponse200ItemsItem>|null
     */
    protected $items;
    /**
     * @var int|null
     */
    protected $page;
    /**
     * @var int|null
     */
    protected $size;
    /**
     * @var int|null
     */
    protected $totalItems;
    /**
     * @var int|null
     */
    protected $totalPages;
    /**
     * @return list<BffEventsGetResponse200ItemsItem>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<BffEventsGetResponse200ItemsItem>|null $items
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
     * @return int|null
     */
    public function getPage(): ?int
    {
        return $this->page;
    }
    /**
     * @param int|null $page
     *
     * @return self
     */
    public function setPage(?int $page): self
    {
        $this->initialized['page'] = true;
        $this->page = $page;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getSize(): ?int
    {
        return $this->size;
    }
    /**
     * @param int|null $size
     *
     * @return self
     */
    public function setSize(?int $size): self
    {
        $this->initialized['size'] = true;
        $this->size = $size;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalItems(): ?int
    {
        return $this->totalItems;
    }
    /**
     * @param int|null $totalItems
     *
     * @return self
     */
    public function setTotalItems(?int $totalItems): self
    {
        $this->initialized['totalItems'] = true;
        $this->totalItems = $totalItems;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotalPages(): ?int
    {
        return $this->totalPages;
    }
    /**
     * @param int|null $totalPages
     *
     * @return self
     */
    public function setTotalPages(?int $totalPages): self
    {
        $this->initialized['totalPages'] = true;
        $this->totalPages = $totalPages;
        return $this;
    }
}