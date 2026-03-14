<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminServiceAccountConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse409
     */
    private $apiAdminServiceAccountsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse409 $apiAdminServiceAccountsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminServiceAccountsPostResponse409 = $apiAdminServiceAccountsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminServiceAccountsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminServiceAccountsPostResponse409
    {
        return $this->apiAdminServiceAccountsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}