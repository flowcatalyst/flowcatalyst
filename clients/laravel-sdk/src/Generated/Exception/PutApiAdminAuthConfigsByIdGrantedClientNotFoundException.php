<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdGrantedClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404
     */
    private $apiAdminAuthConfigsIdGrantedClientsPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404 $apiAdminAuthConfigsIdGrantedClientsPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdGrantedClientsPutResponse404 = $apiAdminAuthConfigsIdGrantedClientsPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdGrantedClientsPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse404
    {
        return $this->apiAdminAuthConfigsIdGrantedClientsPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}