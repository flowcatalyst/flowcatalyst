<?php

namespace FlowCatalyst\Generated\Model;

class FieldError extends \ArrayObject
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
     * Field name
     *
     * @var string|null
     */
    protected $field;
    /**
     * Error message for this field
     *
     * @var string|null
     */
    protected $message;
    /**
     * Field name
     *
     * @return string|null
     */
    public function getField(): ?string
    {
        return $this->field;
    }
    /**
     * Field name
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
     * Error message for this field
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * Error message for this field
     *
     * @param string|null $message
     *
     * @return self
     */
    public function setMessage(?string $message): self
    {
        $this->initialized['message'] = true;
        $this->message = $message;
        return $this;
    }
}