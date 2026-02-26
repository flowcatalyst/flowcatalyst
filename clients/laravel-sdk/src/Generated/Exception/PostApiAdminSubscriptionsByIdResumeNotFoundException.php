<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminSubscriptionsByIdResumeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse404
     */
    private $apiAdminSubscriptionsIdResumePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse404 $apiAdminSubscriptionsIdResumePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdResumePostResponse404 = $apiAdminSubscriptionsIdResumePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdResumePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdResumePostResponse404
    {
        return $this->apiAdminSubscriptionsIdResumePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}