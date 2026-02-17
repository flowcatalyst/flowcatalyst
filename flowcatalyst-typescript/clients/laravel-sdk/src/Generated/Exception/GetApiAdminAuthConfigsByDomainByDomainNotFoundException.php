<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminAuthConfigsByDomainByDomainNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse404
     */
    private $apiAdminAuthConfigsByDomainDomainGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse404 $apiAdminAuthConfigsByDomainDomainGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsByDomainDomainGetResponse404 = $apiAdminAuthConfigsByDomainDomainGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsByDomainDomainGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsByDomainDomainGetResponse404
    {
        return $this->apiAdminAuthConfigsByDomainDomainGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}