<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminConnectionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse404
     */
    private $apiAdminConnectionsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse404 $apiAdminConnectionsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdPutResponse404 = $apiAdminConnectionsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse404
    {
        return $this->apiAdminConnectionsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}