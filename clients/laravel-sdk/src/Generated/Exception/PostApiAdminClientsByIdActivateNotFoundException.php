<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse404
     */
    private $apiAdminClientsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse404 $apiAdminClientsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdActivatePostResponse404 = $apiAdminClientsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse404
    {
        return $this->apiAdminClientsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}