<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdGrantedClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400
     */
    private $apiAdminAuthConfigsIdGrantedClientsPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400 $apiAdminAuthConfigsIdGrantedClientsPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdGrantedClientsPutResponse400 = $apiAdminAuthConfigsIdGrantedClientsPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdGrantedClientsPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGrantedClientsPutResponse400
    {
        return $this->apiAdminAuthConfigsIdGrantedClientsPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}