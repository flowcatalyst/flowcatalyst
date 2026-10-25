<?php

namespace FlowCatalyst\Generated\Model;

class ApplyIngestionSetRequest extends \ArrayObject
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
     * The files the items were read from, as they are in the repository; kept with the apply (at most 2 MB in all)
     *
     * @var list<SetFile>|null
     */
    protected $files;
    /**
     * The set's ingestions, no two of the same application, client and code
     *
     * @var list<IngestionSetItem>|null
     */
    protected $items;
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
     * The files the items were read from, as they are in the repository; kept with the apply (at most 2 MB in all)
     *
     * @return list<SetFile>|null
     */
    public function getFiles(): ?array
    {
        return $this->files;
    }
    /**
     * The files the items were read from, as they are in the repository; kept with the apply (at most 2 MB in all)
     *
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
     * The set's ingestions, no two of the same application, client and code
     *
     * @return list<IngestionSetItem>|null
     */
    public function getItems(): ?array
    {
        return $this->items;
    }
    /**
     * The set's ingestions, no two of the same application, client and code
     *
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
}