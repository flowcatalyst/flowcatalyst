<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse404
     */
    private $apiAdminApplicationsIdClientsPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse404 $apiAdminApplicationsIdClientsPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdClientsPostResponse404 = $apiAdminApplicationsIdClientsPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdClientsPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse404
    {
        return $this->apiAdminApplicationsIdClientsPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}