<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAnchorDomainBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse400
     */
    private $apiAdminAnchorDomainsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse400 $apiAdminAnchorDomainsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsPostResponse400 = $apiAdminAnchorDomainsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsPostResponse400
    {
        return $this->apiAdminAnchorDomainsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}