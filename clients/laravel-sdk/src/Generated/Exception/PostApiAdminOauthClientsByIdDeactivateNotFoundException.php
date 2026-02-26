<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse404
     */
    private $apiAdminOauthClientsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse404 $apiAdminOauthClientsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdDeactivatePostResponse404 = $apiAdminOauthClientsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdDeactivatePostResponse404
    {
        return $this->apiAdminOauthClientsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}