<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdOidcBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse400
     */
    private $apiAdminAuthConfigsIdOidcPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse400 $apiAdminAuthConfigsIdOidcPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdOidcPutResponse400 = $apiAdminAuthConfigsIdOidcPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdOidcPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse400
    {
        return $this->apiAdminAuthConfigsIdOidcPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}