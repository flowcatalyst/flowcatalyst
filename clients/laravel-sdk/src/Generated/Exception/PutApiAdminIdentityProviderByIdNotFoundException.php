<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminIdentityProviderByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse404
     */
    private $apiAdminIdentityProvidersIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse404 $apiAdminIdentityProvidersIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersIdPutResponse404 = $apiAdminIdentityProvidersIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse404
    {
        return $this->apiAdminIdentityProvidersIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}