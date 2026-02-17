<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdConfigTypeBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse400
     */
    private $apiAdminAuthConfigsIdConfigTypePutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse400 $apiAdminAuthConfigsIdConfigTypePutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdConfigTypePutResponse400 = $apiAdminAuthConfigsIdConfigTypePutResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdConfigTypePutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdConfigTypePutResponse400
    {
        return $this->apiAdminAuthConfigsIdConfigTypePutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}