<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminApplicationByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse400
     */
    private $apiAdminApplicationsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse400 $apiAdminApplicationsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdPutResponse400 = $apiAdminApplicationsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdPutResponse400
    {
        return $this->apiAdminApplicationsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}