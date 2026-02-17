<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminServiceAccountByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdDeleteResponse404
     */
    private $apiAdminServiceAccountsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdDeleteResponse404 $apiAdminServiceAccountsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdDeleteResponse404 = $apiAdminServiceAccountsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdDeleteResponse404
    {
        return $this->apiAdminServiceAccountsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}