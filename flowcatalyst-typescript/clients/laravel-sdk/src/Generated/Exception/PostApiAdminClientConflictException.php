<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse409
     */
    private $apiAdminClientsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse409 $apiAdminClientsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsPostResponse409 = $apiAdminClientsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminClientsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminClientsPostResponse409
    {
        return $this->apiAdminClientsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}