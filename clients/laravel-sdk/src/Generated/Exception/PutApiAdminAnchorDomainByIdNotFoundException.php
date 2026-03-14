<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAnchorDomainByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse404
     */
    private $apiAdminAnchorDomainsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse404 $apiAdminAnchorDomainsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsIdPutResponse404 = $apiAdminAnchorDomainsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse404
    {
        return $this->apiAdminAnchorDomainsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}