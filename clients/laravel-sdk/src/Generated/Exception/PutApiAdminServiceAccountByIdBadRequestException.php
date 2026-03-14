<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminServiceAccountByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse400
     */
    private $apiAdminServiceAccountsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse400 $apiAdminServiceAccountsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsIdPutResponse400 = $apiAdminServiceAccountsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsIdPutResponse400
    {
        return $this->apiAdminServiceAccountsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}