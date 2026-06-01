<?php

namespace FlowCatalyst\Generated\Model;

class PaginationParams extends \ArrayObject
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
     * Page number (1-based)
     *
     * @var int|null
     */
    protected $page;
    /**
     * Page size. Aliases: limit, pageSize, page_size.
     *
     * @var int|null
     */
    protected $size;
    /**
     * Page number (1-based)
     *
     * @return int|null
     */
    public function getPage(): ?int
    {
        return $this->page;
    }
    /**
     * Page number (1-based)
     *
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
     * Page size. Aliases: limit, pageSize, page_size.
     *
     * @return int|null
     */
    public function getSize(): ?int
    {
        return $this->size;
    }
    /**
     * Page size. Aliases: limit, pageSize, page_size.
     *
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
}