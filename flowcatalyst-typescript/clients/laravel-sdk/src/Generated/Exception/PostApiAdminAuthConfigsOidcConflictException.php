<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAuthConfigsOidcConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409
     */
    private $apiAdminAuthConfigsOidcPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409 $apiAdminAuthConfigsOidcPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsOidcPostResponse409 = $apiAdminAuthConfigsOidcPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsOidcPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse409
    {
        return $this->apiAdminAuthConfigsOidcPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}