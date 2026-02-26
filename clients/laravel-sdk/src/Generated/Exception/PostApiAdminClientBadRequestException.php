<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse400
     */
    private $apiAdminClientsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse400 $apiAdminClientsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsPostResponse400 = $apiAdminClientsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse400
    {
        return $this->apiAdminClientsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}