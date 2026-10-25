<?php

namespace FlowCatalyst\Generated\Model;

class IngestionFailureListResponse
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
     * @var list<IngestionFailureResponse>|null
     */
    protected $failures;
    /**
     * @var bool|null
     */
    protected $hasMore;
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
     * @return list<IngestionFailureResponse>|null
     */
    public function getFailures(): ?array
    {
        return $this->failures;
    }
    /**
     * @param list<IngestionFailureResponse>|null $failures
     *
     * @return self
     */
    public function setFailures(?array $failures): self
    {
        $this->initialized['failures'] = true;
        $this->failures = $failures;
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