<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAuthConfigsByIdAdditionalClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400
     */
    private $apiAdminAuthConfigsIdAdditionalClientsPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400 $apiAdminAuthConfigsIdAdditionalClientsPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdAdditionalClientsPutResponse400 = $apiAdminAuthConfigsIdAdditionalClientsPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdAdditionalClientsPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdAdditionalClientsPutResponse400
    {
        return $this->apiAdminAuthConfigsIdAdditionalClientsPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}