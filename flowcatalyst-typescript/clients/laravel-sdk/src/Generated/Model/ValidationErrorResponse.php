<?php

namespace FlowCatalyst\Generated\Model;

class ValidationErrorResponse extends \ArrayObject
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
     * Error code
     *
     * @var string|null
     */
    protected $code;
    /**
     * General error message
     *
     * @var string|null
     */
    protected $message;
    /**
     * Field-level validation errors
     *
     * @var list<FieldError>|null
     */
    protected $errors;
    /**
     * Error code
     *
     * @return string|null
     */
    public function getCode(): ?string
    {
        return $this->code;
    }
    /**
     * Error code
     *
     * @param string|null $code
     *
     * @return self
     */
    public function setCode(?string $code): self
    {
        $this->initialized['code'] = true;
        $this->code = $code;
        return $this;
    }
    /**
     * General error message
     *
     * @return string|null
     */
    public function getMessage(): ?string
    {
        return $this->message;
    }
    /**
     * General error message
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
    /**
     * Field-level validation errors
     *
     * @return list<FieldError>|null
     */
    public function getErrors(): ?array
    {
        return $this->errors;
    }
    /**
     * Field-level validation errors
     *
     * @param list<FieldError>|null $errors
     *
     * @return self
     */
    public function setErrors(?array $errors): self
    {
        $this->initialized['errors'] = true;
        $this->errors = $errors;
        return $this;
    }
}