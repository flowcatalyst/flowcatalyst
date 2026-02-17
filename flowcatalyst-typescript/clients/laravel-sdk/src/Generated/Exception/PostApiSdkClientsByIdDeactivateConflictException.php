<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdDeactivateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse409
     */
    private $apiSdkClientsIdDeactivatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse409 $apiSdkClientsIdDeactivatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdDeactivatePostResponse409 = $apiSdkClientsIdDeactivatePostResponse409;
        $this->response = $response;
    }
    public function getApiSdkClientsIdDeactivatePostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdDeactivatePostResponse409
    {
        return $this->apiSdkClientsIdDeactivatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}