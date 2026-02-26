<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse404
     */
    private $apiSdkClientsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse404 $apiSdkClientsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdDeactivatePostResponse404 = $apiSdkClientsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiSdkClientsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse404
    {
        return $this->apiSdkClientsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}