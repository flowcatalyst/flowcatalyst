<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdClientConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse409
     */
    private $apiAdminApplicationsIdClientsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse409 $apiAdminApplicationsIdClientsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdClientsPostResponse409 = $apiAdminApplicationsIdClientsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdClientsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse409
    {
        return $this->apiAdminApplicationsIdClientsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}