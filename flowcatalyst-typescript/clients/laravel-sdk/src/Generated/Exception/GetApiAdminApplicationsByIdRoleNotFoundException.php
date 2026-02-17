<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminApplicationsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse404
     */
    private $apiAdminApplicationsIdRolesGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse404 $apiAdminApplicationsIdRolesGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdRolesGetResponse404 = $apiAdminApplicationsIdRolesGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdRolesGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdRolesGetResponse404
    {
        return $this->apiAdminApplicationsIdRolesGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}