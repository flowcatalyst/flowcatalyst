<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse404
     */
    private $apiSdkClientsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse404 $apiSdkClientsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdActivatePostResponse404 = $apiSdkClientsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiSdkClientsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse404
    {
        return $this->apiSdkClientsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}