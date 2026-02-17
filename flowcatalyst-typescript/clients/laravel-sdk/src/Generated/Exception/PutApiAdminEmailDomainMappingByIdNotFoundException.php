<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminEmailDomainMappingByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse404
     */
    private $apiAdminEmailDomainMappingsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse404 $apiAdminEmailDomainMappingsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsIdPutResponse404 = $apiAdminEmailDomainMappingsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse404
    {
        return $this->apiAdminEmailDomainMappingsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}