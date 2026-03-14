<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientsByIdRotateSecretBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse400
     */
    private $apiAdminOauthClientsIdRotateSecretPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse400 $apiAdminOauthClientsIdRotateSecretPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsIdRotateSecretPostResponse400 = $apiAdminOauthClientsIdRotateSecretPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsIdRotateSecretPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsIdRotateSecretPostResponse400
    {
        return $this->apiAdminOauthClientsIdRotateSecretPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}