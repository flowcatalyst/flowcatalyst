<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeleteResponse404
     */
    private $apiAdminClientsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeleteResponse404 $apiAdminClientsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdDeleteResponse404 = $apiAdminClientsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeleteResponse404
    {
        return $this->apiAdminClientsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}