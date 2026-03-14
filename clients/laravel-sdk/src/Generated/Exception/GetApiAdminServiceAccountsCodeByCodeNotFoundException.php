<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminServiceAccountsCodeByCodeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse404
     */
    private $apiAdminServiceAccountsCodeCodeGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse404 $apiAdminServiceAccountsCodeCodeGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsCodeCodeGetResponse404 = $apiAdminServiceAccountsCodeCodeGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsCodeCodeGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsCodeCodeGetResponse404
    {
        return $this->apiAdminServiceAccountsCodeCodeGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}