<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse404
     */
    private $apiAdminClientsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse404 $apiAdminClientsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdDeactivatePostResponse404 = $apiAdminClientsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse404
    {
        return $this->apiAdminClientsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}