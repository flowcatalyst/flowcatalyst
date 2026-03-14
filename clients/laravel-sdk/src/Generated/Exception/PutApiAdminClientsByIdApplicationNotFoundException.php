<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminClientsByIdApplicationNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse404
     */
    private $apiAdminClientsIdApplicationsPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse404 $apiAdminClientsIdApplicationsPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdApplicationsPutResponse404 = $apiAdminClientsIdApplicationsPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdApplicationsPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsPutResponse404
    {
        return $this->apiAdminClientsIdApplicationsPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}