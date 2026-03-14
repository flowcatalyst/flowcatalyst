<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountsByIdAuthTokenNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse404
     */
    private $apiAdminServiceAccountsIdAuthTokenPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse404 $apiAdminServiceAccountsIdAuthTokenPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdAuthTokenPutResponse404 = $apiAdminServiceAccountsIdAuthTokenPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdAuthTokenPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse404
    {
        return $this->apiAdminServiceAccountsIdAuthTokenPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}