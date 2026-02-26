<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminIdentityProviderBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse400
     */
    private $apiAdminIdentityProvidersPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse400 $apiAdminIdentityProvidersPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersPostResponse400 = $apiAdminIdentityProvidersPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersPostResponse400
    {
        return $this->apiAdminIdentityProvidersPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}