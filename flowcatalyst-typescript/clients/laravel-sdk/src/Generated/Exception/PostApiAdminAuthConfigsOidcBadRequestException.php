<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAuthConfigsOidcBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400
     */
    private $apiAdminAuthConfigsOidcPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400 $apiAdminAuthConfigsOidcPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsOidcPostResponse400 = $apiAdminAuthConfigsOidcPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsOidcPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsOidcPostResponse400
    {
        return $this->apiAdminAuthConfigsOidcPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}