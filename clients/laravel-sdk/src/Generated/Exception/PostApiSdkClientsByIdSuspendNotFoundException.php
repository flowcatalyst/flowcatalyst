<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdSuspendNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse404
     */
    private $apiSdkClientsIdSuspendPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse404 $apiSdkClientsIdSuspendPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdSuspendPostResponse404 = $apiSdkClientsIdSuspendPostResponse404;
        $this->response = $response;
    }
    public function getApiSdkClientsIdSuspendPostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse404
    {
        return $this->apiSdkClientsIdSuspendPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}