<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdSuspendConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse409
     */
    private $apiAdminClientsIdSuspendPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse409 $apiAdminClientsIdSuspendPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdSuspendPostResponse409 = $apiAdminClientsIdSuspendPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminClientsIdSuspendPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdSuspendPostResponse409
    {
        return $this->apiAdminClientsIdSuspendPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}