<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdDeleteResponse404
     */
    private $apiAdminEventTypesIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdDeleteResponse404 $apiAdminEventTypesIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdDeleteResponse404 = $apiAdminEventTypesIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdDeleteResponse404
    {
        return $this->apiAdminEventTypesIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}