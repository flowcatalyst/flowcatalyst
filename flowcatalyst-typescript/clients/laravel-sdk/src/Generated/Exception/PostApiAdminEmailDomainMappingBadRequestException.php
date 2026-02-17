<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEmailDomainMappingBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse400
     */
    private $apiAdminEmailDomainMappingsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse400 $apiAdminEmailDomainMappingsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsPostResponse400 = $apiAdminEmailDomainMappingsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse400
    {
        return $this->apiAdminEmailDomainMappingsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}