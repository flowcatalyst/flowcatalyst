<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkClientByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse400
     */
    private $apiSdkClientsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse400 $apiSdkClientsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdPutResponse400 = $apiSdkClientsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiSdkClientsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdPutResponse400
    {
        return $this->apiSdkClientsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}