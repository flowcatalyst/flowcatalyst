<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminOauthClientByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse400
     */
    private $apiAdminOauthClientsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse400 $apiAdminOauthClientsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdPutResponse400 = $apiAdminOauthClientsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse400
    {
        return $this->apiAdminOauthClientsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}