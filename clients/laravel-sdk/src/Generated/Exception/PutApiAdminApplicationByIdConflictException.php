<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminApplicationByIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse409
     */
    private $apiAdminApplicationsIdPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse409 $apiAdminApplicationsIdPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdPutResponse409 = $apiAdminApplicationsIdPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse409
    {
        return $this->apiAdminApplicationsIdPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}