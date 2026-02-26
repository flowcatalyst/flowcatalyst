<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminClientByIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse409
     */
    private $apiAdminClientsIdPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse409 $apiAdminClientsIdPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdPutResponse409 = $apiAdminClientsIdPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminClientsIdPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdPutResponse409
    {
        return $this->apiAdminClientsIdPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}