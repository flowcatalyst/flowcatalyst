<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse409
     */
    private $apiSdkClientsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse409 $apiSdkClientsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsPostResponse409 = $apiSdkClientsPostResponse409;
        $this->response = $response;
    }
    public function getApiSdkClientsPostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse409
    {
        return $this->apiSdkClientsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}