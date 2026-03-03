<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminConnectionConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse409
     */
    private $apiAdminConnectionsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse409 $apiAdminConnectionsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsPostResponse409 = $apiAdminConnectionsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminConnectionsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse409
    {
        return $this->apiAdminConnectionsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}