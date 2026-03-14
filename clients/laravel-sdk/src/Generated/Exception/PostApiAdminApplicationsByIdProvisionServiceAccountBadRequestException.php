<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdProvisionServiceAccountBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400
     */
    private $apiAdminApplicationsIdProvisionServiceAccountPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400 $apiAdminApplicationsIdProvisionServiceAccountPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse400 = $apiAdminApplicationsIdProvisionServiceAccountPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdProvisionServiceAccountPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdProvisionServiceAccountPostResponse400
    {
        return $this->apiAdminApplicationsIdProvisionServiceAccountPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}