<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAnchorDomainConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse409
     */
    private $apiAdminAnchorDomainsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse409 $apiAdminAnchorDomainsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsPostResponse409 = $apiAdminAnchorDomainsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse409
    {
        return $this->apiAdminAnchorDomainsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}