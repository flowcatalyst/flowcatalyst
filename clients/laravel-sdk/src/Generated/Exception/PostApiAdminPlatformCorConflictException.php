<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPlatformCorConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse409
     */
    private $apiAdminPlatformCorsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse409 $apiAdminPlatformCorsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPlatformCorsPostResponse409 = $apiAdminPlatformCorsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminPlatformCorsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminPlatformCorsPostResponse409
    {
        return $this->apiAdminPlatformCorsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}