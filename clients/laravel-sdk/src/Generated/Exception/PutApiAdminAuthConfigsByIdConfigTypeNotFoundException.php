<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdConfigTypeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse404
     */
    private $apiAdminAuthConfigsIdConfigTypePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse404 $apiAdminAuthConfigsIdConfigTypePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdConfigTypePutResponse404 = $apiAdminAuthConfigsIdConfigTypePutResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdConfigTypePutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse404
    {
        return $this->apiAdminAuthConfigsIdConfigTypePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}