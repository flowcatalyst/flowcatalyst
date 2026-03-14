<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminClientsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminClientsGetResponse200ClientsItem>|null
     */
    protected $clients;
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
     * @return list<ApiAdminClientsGetResponse200ClientsItem>|null
     */
    public function getClients(): ?array
    {
        return $this->clients;
    }
    /**
     * @param list<ApiAdminClientsGetResponse200ClientsItem>|null $clients
     *
     * @return self
     */
    public function setClients(?array $clients): self
    {
        $this->initialized['clients'] = true;
        $this->clients = $clients;
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