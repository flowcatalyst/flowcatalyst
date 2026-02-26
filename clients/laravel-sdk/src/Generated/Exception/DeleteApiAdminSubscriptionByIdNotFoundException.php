<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminSubscriptionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdDeleteResponse404
     */
    private $apiAdminSubscriptionsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdDeleteResponse404 $apiAdminSubscriptionsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdDeleteResponse404 = $apiAdminSubscriptionsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdDeleteResponse404
    {
        return $this->apiAdminSubscriptionsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}