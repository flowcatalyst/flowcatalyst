<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminConnectionsByIdPauseNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPausePostResponse404
     */
    private $apiAdminConnectionsIdPausePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPausePostResponse404 $apiAdminConnectionsIdPausePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdPausePostResponse404 = $apiAdminConnectionsIdPausePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdPausePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPausePostResponse404
    {
        return $this->apiAdminConnectionsIdPausePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}