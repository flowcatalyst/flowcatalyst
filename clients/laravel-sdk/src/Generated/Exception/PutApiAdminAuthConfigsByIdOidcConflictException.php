<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdOidcConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse409
     */
    private $apiAdminAuthConfigsIdOidcPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse409 $apiAdminAuthConfigsIdOidcPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdOidcPutResponse409 = $apiAdminAuthConfigsIdOidcPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdOidcPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdOidcPutResponse409
    {
        return $this->apiAdminAuthConfigsIdOidcPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}