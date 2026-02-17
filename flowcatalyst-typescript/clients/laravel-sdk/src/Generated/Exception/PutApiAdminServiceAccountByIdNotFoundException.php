<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse404
     */
    private $apiAdminServiceAccountsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse404 $apiAdminServiceAccountsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdPutResponse404 = $apiAdminServiceAccountsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse404
    {
        return $this->apiAdminServiceAccountsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}