<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdAdditionalClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404
     */
    private $apiAdminAuthConfigsIdAdditionalClientsPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404 $apiAdminAuthConfigsIdAdditionalClientsPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdAdditionalClientsPutResponse404 = $apiAdminAuthConfigsIdAdditionalClientsPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdAdditionalClientsPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse404
    {
        return $this->apiAdminAuthConfigsIdAdditionalClientsPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}