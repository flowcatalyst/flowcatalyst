<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminIdentityProviderByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse400
     */
    private $apiAdminIdentityProvidersIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse400 $apiAdminIdentityProvidersIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminIdentityProvidersIdPutResponse400 = $apiAdminIdentityProvidersIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminIdentityProvidersIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminIdentityProvidersIdPutResponse400
    {
        return $this->apiAdminIdentityProvidersIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}