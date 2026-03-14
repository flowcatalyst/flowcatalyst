<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminOauthClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse404
     */
    private $apiAdminOauthClientsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse404 $apiAdminOauthClientsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdGetResponse404 = $apiAdminOauthClientsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdGetResponse404
    {
        return $this->apiAdminOauthClientsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}