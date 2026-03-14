<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountsByIdRegenerateSecretNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404
     */
    private $apiAdminServiceAccountsIdRegenerateSecretPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404 $apiAdminServiceAccountsIdRegenerateSecretPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRegenerateSecretPostResponse404 = $apiAdminServiceAccountsIdRegenerateSecretPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRegenerateSecretPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSecretPostResponse404
    {
        return $this->apiAdminServiceAccountsIdRegenerateSecretPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}