<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminIdentityProviderByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdDeleteResponse404
     */
    private $apiAdminIdentityProvidersIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdDeleteResponse404 $apiAdminIdentityProvidersIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersIdDeleteResponse404 = $apiAdminIdentityProvidersIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdDeleteResponse404
    {
        return $this->apiAdminIdentityProvidersIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}