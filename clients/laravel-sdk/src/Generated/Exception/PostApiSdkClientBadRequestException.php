<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse400
     */
    private $apiSdkClientsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse400 $apiSdkClientsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkClientsPostResponse400 = $apiSdkClientsPostResponse400;
        $this->response = $response;
    }
    public function getApiSdkClientsPostResponse400(): \FlowCatalyst\Generated\Model\ApiSdkClientsPostResponse400
    {
        return $this->apiSdkClientsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}