<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountsByIdRegenerateTokenNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404
     */
    private $apiAdminServiceAccountsIdRegenerateTokenPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404 $apiAdminServiceAccountsIdRegenerateTokenPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdRegenerateTokenPostResponse404 = $apiAdminServiceAccountsIdRegenerateTokenPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdRegenerateTokenPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdRegenerateTokenPostResponse404
    {
        return $this->apiAdminServiceAccountsIdRegenerateTokenPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}