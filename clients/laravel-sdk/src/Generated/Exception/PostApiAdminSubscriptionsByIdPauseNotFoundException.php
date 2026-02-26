<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminSubscriptionsByIdPauseNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse404
     */
    private $apiAdminSubscriptionsIdPausePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse404 $apiAdminSubscriptionsIdPausePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdPausePostResponse404 = $apiAdminSubscriptionsIdPausePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdPausePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPausePostResponse404
    {
        return $this->apiAdminSubscriptionsIdPausePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}