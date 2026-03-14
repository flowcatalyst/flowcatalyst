<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminApplicationByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse404
     */
    private $apiAdminApplicationsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse404 $apiAdminApplicationsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdGetResponse404 = $apiAdminApplicationsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdGetResponse404
    {
        return $this->apiAdminApplicationsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}