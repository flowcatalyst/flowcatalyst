<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminEmailDomainMappingByIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse409
     */
    private $apiAdminEmailDomainMappingsIdPutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse409 $apiAdminEmailDomainMappingsIdPutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsIdPutResponse409 = $apiAdminEmailDomainMappingsIdPutResponse409;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsIdPutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse409
    {
        return $this->apiAdminEmailDomainMappingsIdPutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}