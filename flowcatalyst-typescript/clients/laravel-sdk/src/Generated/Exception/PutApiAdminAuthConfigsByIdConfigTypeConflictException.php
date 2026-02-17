<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdConfigTypeConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse409
     */
    private $apiAdminAuthConfigsIdConfigTypePutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse409 $apiAdminAuthConfigsIdConfigTypePutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdConfigTypePutResponse409 = $apiAdminAuthConfigsIdConfigTypePutResponse409;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdConfigTypePutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse409
    {
        return $this->apiAdminAuthConfigsIdConfigTypePutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}