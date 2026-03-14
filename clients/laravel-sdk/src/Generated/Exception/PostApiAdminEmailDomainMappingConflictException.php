<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEmailDomainMappingConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse409
     */
    private $apiAdminEmailDomainMappingsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse409 $apiAdminEmailDomainMappingsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEmailDomainMappingsPostResponse409 = $apiAdminEmailDomainMappingsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminEmailDomainMappingsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminEmailDomainMappingsPostResponse409
    {
        return $this->apiAdminEmailDomainMappingsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}