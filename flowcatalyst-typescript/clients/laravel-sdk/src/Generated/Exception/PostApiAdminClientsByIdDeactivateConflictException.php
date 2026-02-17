<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdDeactivateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse409
     */
    private $apiAdminClientsIdDeactivatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse409 $apiAdminClientsIdDeactivatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdDeactivatePostResponse409 = $apiAdminClientsIdDeactivatePostResponse409;
        $this->response = $response;
    }
    public function getApiAdminClientsIdDeactivatePostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse409
    {
        return $this->apiAdminClientsIdDeactivatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}