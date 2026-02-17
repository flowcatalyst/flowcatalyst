<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminIdentityProviderConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse409
     */
    private $apiAdminIdentityProvidersPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse409 $apiAdminIdentityProvidersPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersPostResponse409 = $apiAdminIdentityProvidersPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse409
    {
        return $this->apiAdminIdentityProvidersPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}