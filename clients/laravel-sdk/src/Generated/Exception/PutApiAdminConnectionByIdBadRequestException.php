<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminConnectionByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse400
     */
    private $apiAdminConnectionsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse400 $apiAdminConnectionsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdPutResponse400 = $apiAdminConnectionsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdPutResponse400
    {
        return $this->apiAdminConnectionsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}