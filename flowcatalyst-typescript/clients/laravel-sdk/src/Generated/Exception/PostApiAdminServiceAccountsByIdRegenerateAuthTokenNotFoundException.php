<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountsByIdRegenerateAuthTokenNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404
     */
    private $apiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404 $apiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404 = $apiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404
    {
        return $this->apiAdminServiceAccountsIdRegenerateAuthTokenPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}