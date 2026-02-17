<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdOidcNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse404
     */
    private $apiAdminAuthConfigsIdOidcPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse404 $apiAdminAuthConfigsIdOidcPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdOidcPutResponse404 = $apiAdminAuthConfigsIdOidcPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdOidcPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse404
    {
        return $this->apiAdminAuthConfigsIdOidcPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}