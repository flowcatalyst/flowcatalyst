<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountsByIdAuthTokenBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse400
     */
    private $apiAdminServiceAccountsIdAuthTokenPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse400 $apiAdminServiceAccountsIdAuthTokenPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdAuthTokenPutResponse400 = $apiAdminServiceAccountsIdAuthTokenPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdAuthTokenPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdAuthTokenPutResponse400
    {
        return $this->apiAdminServiceAccountsIdAuthTokenPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}