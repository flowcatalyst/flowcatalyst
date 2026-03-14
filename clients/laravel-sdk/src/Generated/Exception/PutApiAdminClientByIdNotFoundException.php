<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse404
     */
    private $apiAdminClientsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse404 $apiAdminClientsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdPutResponse404 = $apiAdminClientsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse404
    {
        return $this->apiAdminClientsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}