<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminLoginAttemptsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminLoginAttemptsGetResponse200ItemsItem>|null
     */
    protected $items;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @var int|null
     */
    protected $page;
    /**
     * @var int|null
     */
    protected $pageSize;
    /**
     * @return list<ApiAdminLoginAttemptsGetResponse200ItemsItem>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<ApiAdminLoginAttemptsGetResponse200ItemsItem>|null $items
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
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
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
    public function getPageSize(): ?int
    {
        return $this->pageSize;
    }
    /**
     * @param int|null $pageSize
     *
     * @return self
     */
    public function setPageSize(?int $pageSize): self
    {
        $this->initialized['pageSize'] = true;
        $this->pageSize = $pageSize;
        return $this;
    }
}