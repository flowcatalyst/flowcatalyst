<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminAnchorDomainByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse400
     */
    private $apiAdminAnchorDomainsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse400 $apiAdminAnchorDomainsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAnchorDomainsIdPutResponse400 = $apiAdminAnchorDomainsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminAnchorDomainsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminAnchorDomainsIdPutResponse400
    {
        return $this->apiAdminAnchorDomainsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}