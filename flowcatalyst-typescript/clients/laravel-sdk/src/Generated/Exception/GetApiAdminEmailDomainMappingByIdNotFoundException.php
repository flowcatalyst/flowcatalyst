<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminEmailDomainMappingByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse404
     */
    private $apiAdminEmailDomainMappingsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse404 $apiAdminEmailDomainMappingsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsIdGetResponse404 = $apiAdminEmailDomainMappingsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdGetResponse404
    {
        return $this->apiAdminEmailDomainMappingsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}