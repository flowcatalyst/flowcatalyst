<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiSdkClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse404
     */
    private $apiSdkClientsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse404 $apiSdkClientsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdGetResponse404 = $apiSdkClientsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiSdkClientsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdGetResponse404
    {
        return $this->apiSdkClientsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}