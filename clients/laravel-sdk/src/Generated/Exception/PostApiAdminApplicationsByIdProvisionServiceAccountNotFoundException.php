<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdProvisionServiceAccountNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404
     */
    private $apiAdminApplicationsIdProvisionServiceAccountPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404 $apiAdminApplicationsIdProvisionServiceAccountPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse404 = $apiAdminApplicationsIdProvisionServiceAccountPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdProvisionServiceAccountPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse404
    {
        return $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}