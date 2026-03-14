<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminSubscriptionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse404
     */
    private $apiAdminSubscriptionsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse404 $apiAdminSubscriptionsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdGetResponse404 = $apiAdminSubscriptionsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdGetResponse404
    {
        return $this->apiAdminSubscriptionsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}