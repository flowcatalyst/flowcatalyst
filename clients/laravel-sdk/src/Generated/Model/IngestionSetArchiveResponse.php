<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSetArchiveResponse
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
     * @var \DateTime|null
     */
    protected $appliedAt;
    /**
     * @var string|null
     */
    protected $appliedBy;
    /**
     * @var string|null
     */
    protected $applyId;
    /**
     * @var list<SetFile>|null
     */
    protected $files;
    /**
     * @var string|null
     */
    protected $goingBackTo;
    /**
     * @var list<IngestionSetItem>|null
     */
    protected $items;
    /**
     * @var string|null
     */
    protected $set;
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
     * @return \DateTime|null
     */
    public function getAppliedAt(): ?\DateTime
    {
        return $this->appliedAt;
    }
    /**
     * @param \DateTime|null $appliedAt
     *
     * @return self
     */
    public function setAppliedAt(?\DateTime $appliedAt): self
    {
        $this->initialized['appliedAt'] = true;
        $this->appliedAt = $appliedAt;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getAppliedBy(): ?string
    {
        return $this->appliedBy;
    }
    /**
     * @param string|null $appliedBy
     *
     * @return self
     */
    public function setAppliedBy(?string $appliedBy): self
    {
        $this->initialized['appliedBy'] = true;
        $this->appliedBy = $appliedBy;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getApplyId(): ?string
    {
        return $this->applyId;
    }
    /**
     * @param string|null $applyId
     *
     * @return self
     */
    public function setApplyId(?string $applyId): self
    {
        $this->initialized['applyId'] = true;
        $this->applyId = $applyId;
        return $this;
    }
    /**
     * @return list<SetFile>|null
     */
    public function getFiles(): ?array
    {
        return $this->files;
    }
    /**
     * @param list<SetFile>|null $files
     *
     * @return self
     */
    public function setFiles(?array $files): self
    {
        $this->initialized['files'] = true;
        $this->files = $files;
        return $this;
    }
    /**
     * @return string|null
     */
    public function getGoingBackTo(): ?string
    {
        return $this->goingBackTo;
    }
    /**
     * @param string|null $goingBackTo
     *
     * @return self
     */
    public function setGoingBackTo(?string $goingBackTo): self
    {
        $this->initialized['goingBackTo'] = true;
        $this->goingBackTo = $goingBackTo;
        return $this;
    }
    /**
     * @return list<IngestionSetItem>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * @param list<IngestionSetItem>|null $items
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
     * @return string|null
     */
    public function getSet(): ?string
    {
        return $this->set;
    }
    /**
     * @param string|null $set
     *
     * @return self
     */
    public function setSet(?string $set): self
    {
        $this->initialized['set'] = true;
        $this->set = $set;
        return $this;
    }
}