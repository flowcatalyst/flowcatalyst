<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdActivateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse409
     */
    private $apiAdminClientsIdActivatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse409 $apiAdminClientsIdActivatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdActivatePostResponse409 = $apiAdminClientsIdActivatePostResponse409;
        $this->response = $response;
    }
    public function getApiAdminClientsIdActivatePostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse409
    {
        return $this->apiAdminClientsIdActivatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}