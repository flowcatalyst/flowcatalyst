<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminAnchorDomainByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse404
     */
    private $apiAdminAnchorDomainsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse404 $apiAdminAnchorDomainsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsIdGetResponse404 = $apiAdminAnchorDomainsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdGetResponse404
    {
        return $this->apiAdminAnchorDomainsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}