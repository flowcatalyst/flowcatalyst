<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse404
     */
    private $apiAdminClientsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse404 $apiAdminClientsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdGetResponse404 = $apiAdminClientsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdGetResponse404
    {
        return $this->apiAdminClientsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}