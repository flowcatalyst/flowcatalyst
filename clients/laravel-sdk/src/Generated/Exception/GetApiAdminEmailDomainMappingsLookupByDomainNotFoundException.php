<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminEmailDomainMappingsLookupByDomainNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse404
     */
    private $apiAdminEmailDomainMappingsLookupDomainGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse404 $apiAdminEmailDomainMappingsLookupDomainGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsLookupDomainGetResponse404 = $apiAdminEmailDomainMappingsLookupDomainGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsLookupDomainGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsLookupDomainGetResponse404
    {
        return $this->apiAdminEmailDomainMappingsLookupDomainGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}