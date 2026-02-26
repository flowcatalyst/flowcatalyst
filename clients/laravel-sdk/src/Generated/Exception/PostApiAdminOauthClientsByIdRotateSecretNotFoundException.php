<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdRotateSecretNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse404
     */
    private $apiAdminOauthClientsIdRotateSecretPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse404 $apiAdminOauthClientsIdRotateSecretPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdRotateSecretPostResponse404 = $apiAdminOauthClientsIdRotateSecretPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdRotateSecretPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse404
    {
        return $this->apiAdminOauthClientsIdRotateSecretPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}