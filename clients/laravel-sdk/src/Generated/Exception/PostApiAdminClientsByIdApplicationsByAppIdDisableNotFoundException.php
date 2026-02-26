<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdApplicationsByAppIdDisableNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404
     */
    private $apiAdminClientsIdApplicationsAppIdDisablePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404 $apiAdminClientsIdApplicationsAppIdDisablePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdApplicationsAppIdDisablePostResponse404 = $apiAdminClientsIdApplicationsAppIdDisablePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminClientsIdApplicationsAppIdDisablePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdApplicationsAppIdDisablePostResponse404
    {
        return $this->apiAdminClientsIdApplicationsAppIdDisablePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}