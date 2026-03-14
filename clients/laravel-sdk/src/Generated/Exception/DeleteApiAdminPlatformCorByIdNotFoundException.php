<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminPlatformCorByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdDeleteResponse404
     */
    private $apiAdminPlatformCorsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdDeleteResponse404 $apiAdminPlatformCorsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPlatformCorsIdDeleteResponse404 = $apiAdminPlatformCorsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminPlatformCorsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsIdDeleteResponse404
    {
        return $this->apiAdminPlatformCorsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}