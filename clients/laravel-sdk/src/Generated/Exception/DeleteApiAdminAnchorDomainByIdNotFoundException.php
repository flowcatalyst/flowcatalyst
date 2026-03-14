<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminAnchorDomainByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdDeleteResponse404
     */
    private $apiAdminAnchorDomainsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdDeleteResponse404 $apiAdminAnchorDomainsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsIdDeleteResponse404 = $apiAdminAnchorDomainsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdDeleteResponse404
    {
        return $this->apiAdminAnchorDomainsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}