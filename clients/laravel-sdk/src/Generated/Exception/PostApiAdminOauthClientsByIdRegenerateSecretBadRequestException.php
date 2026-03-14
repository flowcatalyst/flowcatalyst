<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdRegenerateSecretBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse400
     */
    private $apiAdminOauthClientsIdRegenerateSecretPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse400 $apiAdminOauthClientsIdRegenerateSecretPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdRegenerateSecretPostResponse400 = $apiAdminOauthClientsIdRegenerateSecretPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdRegenerateSecretPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRegenerateSecretPostResponse400
    {
        return $this->apiAdminOauthClientsIdRegenerateSecretPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}