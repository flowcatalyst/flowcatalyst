<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse404
     */
    private $apiAdminApplicationsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse404 $apiAdminApplicationsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdActivatePostResponse404 = $apiAdminApplicationsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdActivatePostResponse404
    {
        return $this->apiAdminApplicationsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}