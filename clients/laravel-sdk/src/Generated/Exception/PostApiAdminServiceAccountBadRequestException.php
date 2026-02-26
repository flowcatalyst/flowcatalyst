<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse400
     */
    private $apiAdminServiceAccountsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse400 $apiAdminServiceAccountsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsPostResponse400 = $apiAdminServiceAccountsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse400
    {
        return $this->apiAdminServiceAccountsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}