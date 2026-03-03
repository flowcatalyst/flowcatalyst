<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminConnectionsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdActivatePostResponse404
     */
    private $apiAdminConnectionsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdActivatePostResponse404 $apiAdminConnectionsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdActivatePostResponse404 = $apiAdminConnectionsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdActivatePostResponse404
    {
        return $this->apiAdminConnectionsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}