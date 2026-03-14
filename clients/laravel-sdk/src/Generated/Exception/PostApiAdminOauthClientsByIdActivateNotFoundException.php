<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse404
     */
    private $apiAdminOauthClientsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse404 $apiAdminOauthClientsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdActivatePostResponse404 = $apiAdminOauthClientsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdActivatePostResponse404
    {
        return $this->apiAdminOauthClientsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}