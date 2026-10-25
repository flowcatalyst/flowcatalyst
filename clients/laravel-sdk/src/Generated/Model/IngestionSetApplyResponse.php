<?php

namespace FlowCatalyst\Generated\Model;

class IngestionSetApplyResponse
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
     * @var \DateTime|null
     */
    protected $appliedAt;
    /**
     * @var string|null
     */
    protected $appliedBy;
    /**
     * @var list<SetChange>|null
     */
    protected $changes;
    /**
     * How many files they were read from
     *
     * @var int|null
     */
    protected $files;
    /**
     * The earlier apply it applied again
     *
     * @var string|null
     */
    protected $goingBackTo;
    /**
     * @var string|null
     */
    protected $id;
    /**
     * How many items it was given
     *
     * @var int|null
     */
    protected $items;
    /**
     * @var string|null
     */
    protected $set;
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
     * @return list<SetChange>|null
     */
    public function getChanges(): ?array
    {
        return $this->changes;
    }
    /**
     * @param list<SetChange>|null $changes
     *
     * @return self
     */
    public function setChanges(?array $changes): self
    {
        $this->initialized['changes'] = true;
        $this->changes = $changes;
        return $this;
    }
    /**
     * How many files they were read from
     *
     * @return int|null
     */
    public function getFiles(): ?int
    {
        return $this->files;
    }
    /**
     * How many files they were read from
     *
     * @param int|null $files
     *
     * @return self
     */
    public function setFiles(?int $files): self
    {
        $this->initialized['files'] = true;
        $this->files = $files;
        return $this;
    }
    /**
     * The earlier apply it applied again
     *
     * @return string|null
     */
    public function getGoingBackTo(): ?string
    {
        return $this->goingBackTo;
    }
    /**
     * The earlier apply it applied again
     *
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
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * @param string|null $id
     *
     * @return self
     */
    public function setId(?string $id): self
    {
        $this->initialized['id'] = true;
        $this->id = $id;
        return $this;
    }
    /**
     * How many items it was given
     *
     * @return int|null
     */
    public function getItems(): ?int
    {
        return $this->items;
    }
    /**
     * How many items it was given
     *
     * @param int|null $items
     *
     * @return self
     */
    public function setItems(?int $items): self
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