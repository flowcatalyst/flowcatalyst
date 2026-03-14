<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminApplicationsByCodeByCodeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404
     */
    private $apiAdminApplicationsByCodeCodeGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404 $apiAdminApplicationsByCodeCodeGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsByCodeCodeGetResponse404 = $apiAdminApplicationsByCodeCodeGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminApplicationsByCodeCodeGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsByCodeCodeGetResponse404
    {
        return $this->apiAdminApplicationsByCodeCodeGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}