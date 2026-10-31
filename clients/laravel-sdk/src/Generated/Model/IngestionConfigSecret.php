<?php

namespace FlowCatalyst\Generated\Model;

class IngestionConfigSecret
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
     * The path of the setting in the entry, such as password or auth.clientSecret
     *
     * @var string|null
     */
    protected $field;
    /**
     * The name of the secret the entry gives the setting; absent when it names none
     *
     * @var string|null
     */
    protected $name;
    /**
     * The secret has a value at this client
     *
     * @var bool|null
     */
    protected $set;
    /**
     * When the value was last set, RFC 3339; absent when not set
     *
     * @var string|null
     */
    protected $updatedAt;
    /**
     * The path of the setting in the entry, such as password or auth.clientSecret
     *
     * @return string|null
     */
    public function getField(): ?string
    {
        return $this->field;
    }
    /**
     * The path of the setting in the entry, such as password or auth.clientSecret
     *
     * @param string|null $field
     *
     * @return self
     */
    public function setField(?string $field): self
    {
        $this->initialized['field'] = true;
        $this->field = $field;
        return $this;
    }
    /**
     * The name of the secret the entry gives the setting; absent when it names none
     *
     * @return string|null
     */
    public function getName(): ?string
    {
        return $this->name;
    }
    /**
     * The name of the secret the entry gives the setting; absent when it names none
     *
     * @param string|null $name
     *
     * @return self
     */
    public function setName(?string $name): self
    {
        $this->initialized['name'] = true;
        $this->name = $name;
        return $this;
    }
    /**
     * The secret has a value at this client
     *
     * @return bool|null
     */
    public function getSet(): ?bool
    {
        return $this->set;
    }
    /**
     * The secret has a value at this client
     *
     * @param bool|null $set
     *
     * @return self
     */
    public function setSet(?bool $set): self
    {
        $this->initialized['set'] = true;
        $this->set = $set;
        return $this;
    }
    /**
     * When the value was last set, RFC 3339; absent when not set
     *
     * @return string|null
     */
    public function getUpdatedAt(): ?string
    {
        return $this->updatedAt;
    }
    /**
     * When the value was last set, RFC 3339; absent when not set
     *
     * @param string|null $updatedAt
     *
     * @return self
     */
    public function setUpdatedAt(?string $updatedAt): self
    {
        $this->initialized['updatedAt'] = true;
        $this->updatedAt = $updatedAt;
        return $this;
    }
}