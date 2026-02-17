<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminAuthConfigByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse404
     */
    private $apiAdminAuthConfigsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse404 $apiAdminAuthConfigsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdGetResponse404 = $apiAdminAuthConfigsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdGetResponse404
    {
        return $this->apiAdminAuthConfigsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}