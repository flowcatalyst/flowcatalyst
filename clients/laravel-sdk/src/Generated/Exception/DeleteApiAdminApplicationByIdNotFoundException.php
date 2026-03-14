<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminApplicationByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeleteResponse404
     */
    private $apiAdminApplicationsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeleteResponse404 $apiAdminApplicationsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdDeleteResponse404 = $apiAdminApplicationsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeleteResponse404
    {
        return $this->apiAdminApplicationsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}