<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminSubscriptionConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse409
     */
    private $apiAdminSubscriptionsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse409 $apiAdminSubscriptionsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsPostResponse409 = $apiAdminSubscriptionsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse409
    {
        return $this->apiAdminSubscriptionsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}