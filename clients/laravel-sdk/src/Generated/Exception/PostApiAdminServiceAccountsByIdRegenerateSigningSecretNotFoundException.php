<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountsByIdRegenerateSigningSecretNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404
     */
    private $apiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404 $apiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404 = $apiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404
    {
        return $this->apiAdminServiceAccountsIdRegenerateSigningSecretPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}