<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientsByIdSuspendConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse409
     */
    private $apiSdkClientsIdSuspendPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse409 $apiSdkClientsIdSuspendPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsIdSuspendPostResponse409 = $apiSdkClientsIdSuspendPostResponse409;
        $this->response = $response;
    }
    public function getApiSdkClientsIdSuspendPostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkClientsIdSuspendPostResponse409
    {
        return $this->apiSdkClientsIdSuspendPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}