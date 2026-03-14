<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdApplicationsByAppIdEnableNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404
     */
    private $apiAdminClientsIdApplicationsAppIdEnablePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404 $apiAdminClientsIdApplicationsAppIdEnablePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdApplicationsAppIdEnablePostResponse404 = $apiAdminClientsIdApplicationsAppIdEnablePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdApplicationsAppIdEnablePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdEnablePostResponse404
    {
        return $this->apiAdminClientsIdApplicationsAppIdEnablePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}