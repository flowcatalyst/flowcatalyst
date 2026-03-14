<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse404
     */
    private $apiAdminEventTypesIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse404 $apiAdminEventTypesIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdGetResponse404 = $apiAdminEventTypesIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdGetResponse404
    {
        return $this->apiAdminEventTypesIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}