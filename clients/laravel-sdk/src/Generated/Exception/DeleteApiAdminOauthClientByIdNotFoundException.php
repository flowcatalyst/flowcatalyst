<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminOauthClientByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeleteResponse404
     */
    private $apiAdminOauthClientsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeleteResponse404 $apiAdminOauthClientsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdDeleteResponse404 = $apiAdminOauthClientsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeleteResponse404
    {
        return $this->apiAdminOauthClientsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}