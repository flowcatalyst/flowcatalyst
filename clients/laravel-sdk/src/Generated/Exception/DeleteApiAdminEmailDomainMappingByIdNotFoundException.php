<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminEmailDomainMappingByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdDeleteResponse404
     */
    private $apiAdminEmailDomainMappingsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdDeleteResponse404 $apiAdminEmailDomainMappingsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsIdDeleteResponse404 = $apiAdminEmailDomainMappingsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsIdDeleteResponse404
    {
        return $this->apiAdminEmailDomainMappingsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}