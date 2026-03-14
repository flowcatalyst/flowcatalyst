<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse400
     */
    private $apiAdminApplicationsIdClientsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse400 $apiAdminApplicationsIdClientsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdClientsPostResponse400 = $apiAdminApplicationsIdClientsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdClientsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdClientsPostResponse400
    {
        return $this->apiAdminApplicationsIdClientsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}