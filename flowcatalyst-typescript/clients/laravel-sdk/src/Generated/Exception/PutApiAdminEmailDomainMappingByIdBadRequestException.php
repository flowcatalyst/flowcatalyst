<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminEmailDomainMappingByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse400
     */
    private $apiAdminEmailDomainMappingsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse400 $apiAdminEmailDomainMappingsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsIdPutResponse400 = $apiAdminEmailDomainMappingsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdPutResponse400
    {
        return $this->apiAdminEmailDomainMappingsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}