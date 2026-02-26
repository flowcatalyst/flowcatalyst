<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminApplicationsByIdClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse404
     */
    private $apiAdminApplicationsIdClientsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse404 $apiAdminApplicationsIdClientsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdClientsGetResponse404 = $apiAdminApplicationsIdClientsGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdClientsGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsGetResponse404
    {
        return $this->apiAdminApplicationsIdClientsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}