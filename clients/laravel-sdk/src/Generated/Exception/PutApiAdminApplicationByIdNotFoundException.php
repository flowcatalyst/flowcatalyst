<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminApplicationByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse404
     */
    private $apiAdminApplicationsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse404 $apiAdminApplicationsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdPutResponse404 = $apiAdminApplicationsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse404
    {
        return $this->apiAdminApplicationsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}