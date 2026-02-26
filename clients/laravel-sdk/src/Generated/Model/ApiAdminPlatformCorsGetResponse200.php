<?php

namespace FlowCatalyst\Generated\Model;

class ApiAdminPlatformCorsGetResponse200 extends \ArrayObject
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
     * @var list<ApiAdminPlatformCorsGetResponse200CorsOriginsItem>|null
     */
    protected $corsOrigins;
    /**
     * @var int|null
     */
    protected $total;
    /**
     * @return list<ApiAdminPlatformCorsGetResponse200CorsOriginsItem>|null
     */
    public function getCorsOrigins(): ?array
    {
        return $this->corsOrigins;
    }
    /**
     * @param list<ApiAdminPlatformCorsGetResponse200CorsOriginsItem>|null $corsOrigins
     *
     * @return self
     */
    public function setCorsOrigins(?array $corsOrigins): self
    {
        $this->initialized['corsOrigins'] = true;
        $this->corsOrigins = $corsOrigins;
        return $this;
    }
    /**
     * @return int|null
     */
    public function getTotal(): ?int
    {
        return $this->total;
    }
    /**
     * @param int|null $total
     *
     * @return self
     */
    public function setTotal(?int $total): self
    {
        $this->initialized['total'] = true;
        $this->total = $total;
        return $this;
    }
}