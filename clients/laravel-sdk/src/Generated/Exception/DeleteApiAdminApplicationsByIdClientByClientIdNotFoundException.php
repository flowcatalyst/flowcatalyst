<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminApplicationsByIdClientByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsClientIdDeleteResponse404
     */
    private $apiAdminApplicationsIdClientsClientIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsClientIdDeleteResponse404 $apiAdminApplicationsIdClientsClientIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdClientsClientIdDeleteResponse404 = $apiAdminApplicationsIdClientsClientIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdClientsClientIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsClientIdDeleteResponse404
    {
        return $this->apiAdminApplicationsIdClientsClientIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}