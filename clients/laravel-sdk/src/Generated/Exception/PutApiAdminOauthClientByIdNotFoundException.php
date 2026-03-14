<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminOauthClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse404
     */
    private $apiAdminOauthClientsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse404 $apiAdminOauthClientsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdPutResponse404 = $apiAdminOauthClientsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse404
    {
        return $this->apiAdminOauthClientsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}