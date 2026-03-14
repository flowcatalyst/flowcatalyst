<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminClientsByIdApplicationNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse404
     */
    private $apiAdminClientsIdApplicationsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse404 $apiAdminClientsIdApplicationsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdApplicationsGetResponse404 = $apiAdminClientsIdApplicationsGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdApplicationsGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsGetResponse404
    {
        return $this->apiAdminClientsIdApplicationsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}