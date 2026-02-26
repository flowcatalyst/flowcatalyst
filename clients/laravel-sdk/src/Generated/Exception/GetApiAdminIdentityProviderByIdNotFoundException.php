<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminIdentityProviderByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse404
     */
    private $apiAdminIdentityProvidersIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse404 $apiAdminIdentityProvidersIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersIdGetResponse404 = $apiAdminIdentityProvidersIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdGetResponse404
    {
        return $this->apiAdminIdentityProvidersIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}