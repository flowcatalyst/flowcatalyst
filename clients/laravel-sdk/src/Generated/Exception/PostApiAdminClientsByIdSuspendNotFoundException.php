<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdSuspendNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse404
     */
    private $apiAdminClientsIdSuspendPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse404 $apiAdminClientsIdSuspendPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdSuspendPostResponse404 = $apiAdminClientsIdSuspendPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdSuspendPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse404
    {
        return $this->apiAdminClientsIdSuspendPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}