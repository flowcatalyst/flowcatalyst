<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdSuspendBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse400
     */
    private $apiAdminClientsIdSuspendPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse400 $apiAdminClientsIdSuspendPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdSuspendPostResponse400 = $apiAdminClientsIdSuspendPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsIdSuspendPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse400
    {
        return $this->apiAdminClientsIdSuspendPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}