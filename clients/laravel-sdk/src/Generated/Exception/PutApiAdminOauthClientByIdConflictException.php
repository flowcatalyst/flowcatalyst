<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminOauthClientByIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse409
     */
    private $apiAdminOauthClientsIdPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse409 $apiAdminOauthClientsIdPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdPutResponse409 = $apiAdminOauthClientsIdPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdPutResponse409
    {
        return $this->apiAdminOauthClientsIdPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}