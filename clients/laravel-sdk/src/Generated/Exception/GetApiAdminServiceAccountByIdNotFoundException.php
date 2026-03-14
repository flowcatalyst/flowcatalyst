<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminServiceAccountByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse404
     */
    private $apiAdminServiceAccountsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse404 $apiAdminServiceAccountsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdGetResponse404 = $apiAdminServiceAccountsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdGetResponse404
    {
        return $this->apiAdminServiceAccountsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}