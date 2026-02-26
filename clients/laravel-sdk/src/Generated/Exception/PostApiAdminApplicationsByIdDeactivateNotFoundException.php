<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse404
     */
    private $apiAdminApplicationsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse404 $apiAdminApplicationsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdDeactivatePostResponse404 = $apiAdminApplicationsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse404
    {
        return $this->apiAdminApplicationsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}