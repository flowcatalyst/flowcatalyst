<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminAuthConfigByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdDeleteResponse404
     */
    private $apiAdminAuthConfigsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdDeleteResponse404 $apiAdminAuthConfigsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsIdDeleteResponse404 = $apiAdminAuthConfigsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsIdDeleteResponse404
    {
        return $this->apiAdminAuthConfigsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}