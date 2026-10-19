<?php

namespace FlowCatalyst\Generated\Model;

class EditPayloadRequest extends \ArrayObject
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
     * The new payload text (UTF-8, at most 256 KiB; valid JSON when the content type is JSON)
     *
     * @var string|null
     */
    protected $payload;
    /**
     * New media type; omit to keep the job's
     *
     * @var string|null
     */
    protected $payloadContentType;
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
     * The new payload text (UTF-8, at most 256 KiB; valid JSON when the content type is JSON)
     *
     * @return string|null
     */
    public function getPayload(): ?string
    {
        return $this->payload;
    }
    /**
     * The new payload text (UTF-8, at most 256 KiB; valid JSON when the content type is JSON)
     *
     * @param string|null $payload
     *
     * @return self
     */
    public function setPayload(?string $payload): self
    {
        $this->initialized['payload'] = true;
        $this->payload = $payload;
        return $this;
    }
    /**
     * New media type; omit to keep the job's
     *
     * @return string|null
     */
    public function getPayloadContentType(): ?string
    {
        return $this->payloadContentType;
    }
    /**
     * New media type; omit to keep the job's
     *
     * @param string|null $payloadContentType
     *
     * @return self
     */
    public function setPayloadContentType(?string $payloadContentType): self
    {
        $this->initialized['payloadContentType'] = true;
        $this->payloadContentType = $payloadContentType;
        return $this;
    }
}