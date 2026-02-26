<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPlatformCorByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse404
     */
    private $apiAdminPlatformCorsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse404 $apiAdminPlatformCorsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPlatformCorsIdGetResponse404 = $apiAdminPlatformCorsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPlatformCorsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdGetResponse404
    {
        return $this->apiAdminPlatformCorsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}