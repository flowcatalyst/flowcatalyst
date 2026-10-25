<?php

namespace FlowCatalyst\Generated\Model;

class IngestionListResponse
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
     * @var bool|null
     */
    protected $hasMore;
    /**
     * @var list<IngestionResponse>|null
     */
    protected $ingestions;
    /**
     * Pass as after to read the next page; absent on the last
     *
     * @var string|null
     */
    protected $nextCursor;
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
     * @return bool|null
     */
    public function getHasMore(): ?bool
    {
        return $this->hasMore;
    }
    /**
     * @param bool|null $hasMore
     *
     * @return self
     */
    public function setHasMore(?bool $hasMore): self
    {
        $this->initialized['hasMore'] = true;
        $this->hasMore = $hasMore;
        return $this;
    }
    /**
     * @return list<IngestionResponse>|null
     */
    public function getIngestions(): ?array
    {
        return $this->ingestions;
    }
    /**
     * @param list<IngestionResponse>|null $ingestions
     *
     * @return self
     */
    public function setIngestions(?array $ingestions): self
    {
        $this->initialized['ingestions'] = true;
        $this->ingestions = $ingestions;
        return $this;
    }
    /**
     * Pass as after to read the next page; absent on the last
     *
     * @return string|null
     */
    public function getNextCursor(): ?string
    {
        return $this->nextCursor;
    }
    /**
     * Pass as after to read the next page; absent on the last
     *
     * @param string|null $nextCursor
     *
     * @return self
     */
    public function setNextCursor(?string $nextCursor): self
    {
        $this->initialized['nextCursor'] = true;
        $this->nextCursor = $nextCursor;
        return $this;
    }
}