<?php

namespace FlowCatalyst\Generated\Model;

class CorsOriginDto extends \ArrayObject
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
     * Origin ID
     *
     * @var string|null
     */
    protected $id;
    /**
     * Origin URL
     *
     * @var string|null
     */
    protected $origin;
    /**
     * Description of why this origin is allowed
     *
     * @var string|null
     */
    protected $description;
    /**
     * Principal ID who created this entry
     *
     * @var string|null
     */
    protected $createdBy;
    /**
     * When this entry was created
     *
     * @var string|null
     */
    protected $createdAt;
    /**
     * Origin ID
     *
     * @return string|null
     */
    public function getId(): ?string
    {
        return $this->id;
    }
    /**
     * Origin ID
     *
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
     * Origin URL
     *
     * @return string|null
     */
    public function getOrigin(): ?string
    {
        return $this->origin;
    }
    /**
     * Origin URL
     *
     * @param string|null $origin
     *
     * @return self
     */
    public function setOrigin(?string $origin): self
    {
        $this->initialized['origin'] = true;
        $this->origin = $origin;
        return $this;
    }
    /**
     * Description of why this origin is allowed
     *
     * @return string|null
     */
    public function getDescription(): ?string
    {
        return $this->description;
    }
    /**
     * Description of why this origin is allowed
     *
     * @param string|null $description
     *
     * @return self
     */
    public function setDescription(?string $description): self
    {
        $this->initialized['description'] = true;
        $this->description = $description;
        return $this;
    }
    /**
     * Principal ID who created this entry
     *
     * @return string|null
     */
    public function getCreatedBy(): ?string
    {
        return $this->createdBy;
    }
    /**
     * Principal ID who created this entry
     *
     * @param string|null $createdBy
     *
     * @return self
     */
    public function setCreatedBy(?string $createdBy): self
    {
        $this->initialized['createdBy'] = true;
        $this->createdBy = $createdBy;
        return $this;
    }
    /**
     * When this entry was created
     *
     * @return string|null
     */
    public function getCreatedAt(): ?string
    {
        return $this->createdAt;
    }
    /**
     * When this entry was created
     *
     * @param string|null $createdAt
     *
     * @return self
     */
    public function setCreatedAt(?string $createdAt): self
    {
        $this->initialized['createdAt'] = true;
        $this->createdAt = $createdAt;
        return $this;
    }
}