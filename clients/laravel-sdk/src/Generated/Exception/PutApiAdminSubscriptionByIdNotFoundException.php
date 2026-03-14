<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminSubscriptionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse404
     */
    private $apiAdminSubscriptionsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse404 $apiAdminSubscriptionsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdPutResponse404 = $apiAdminSubscriptionsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse404
    {
        return $this->apiAdminSubscriptionsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}