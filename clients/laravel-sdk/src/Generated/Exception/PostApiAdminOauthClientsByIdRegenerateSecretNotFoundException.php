<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdRegenerateSecretNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse404
     */
    private $apiAdminOauthClientsIdRegenerateSecretPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse404 $apiAdminOauthClientsIdRegenerateSecretPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdRegenerateSecretPostResponse404 = $apiAdminOauthClientsIdRegenerateSecretPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdRegenerateSecretPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse404
    {
        return $this->apiAdminOauthClientsIdRegenerateSecretPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}