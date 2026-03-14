<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdActivateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse409
     */
    private $apiSdkClientsIdActivatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse409 $apiSdkClientsIdActivatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdActivatePostResponse409 = $apiSdkClientsIdActivatePostResponse409;
        $this->response = $response;
    }
    public function getApiSdkClientsIdActivatePostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdActivatePostResponse409
    {
        return $this->apiSdkClientsIdActivatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}