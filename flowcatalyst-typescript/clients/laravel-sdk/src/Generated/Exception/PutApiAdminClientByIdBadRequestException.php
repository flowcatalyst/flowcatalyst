<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminClientByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse400
     */
    private $apiAdminClientsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse400 $apiAdminClientsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdPutResponse400 = $apiAdminClientsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse400
    {
        return $this->apiAdminClientsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}