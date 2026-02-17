<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPlatformCorBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse400
     */
    private $apiAdminPlatformCorsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse400 $apiAdminPlatformCorsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPlatformCorsPostResponse400 = $apiAdminPlatformCorsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminPlatformCorsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse400
    {
        return $this->apiAdminPlatformCorsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}