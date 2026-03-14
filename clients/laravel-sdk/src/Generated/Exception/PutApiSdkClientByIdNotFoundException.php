<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse404
     */
    private $apiSdkClientsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse404 $apiSdkClientsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdPutResponse404 = $apiSdkClientsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiSdkClientsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse404
    {
        return $this->apiSdkClientsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}