<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminIdentityProviderByIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse409
     */
    private $apiAdminIdentityProvidersIdPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse409 $apiAdminIdentityProvidersIdPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersIdPutResponse409 = $apiAdminIdentityProvidersIdPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersIdPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse409
    {
        return $this->apiAdminIdentityProvidersIdPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}