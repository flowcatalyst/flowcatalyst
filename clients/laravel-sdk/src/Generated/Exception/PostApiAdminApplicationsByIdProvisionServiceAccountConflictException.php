<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdProvisionServiceAccountConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409
     */
    private $apiAdminApplicationsIdProvisionServiceAccountPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409 $apiAdminApplicationsIdProvisionServiceAccountPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse409 = $apiAdminApplicationsIdProvisionServiceAccountPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdProvisionServiceAccountPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse409
    {
        return $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}