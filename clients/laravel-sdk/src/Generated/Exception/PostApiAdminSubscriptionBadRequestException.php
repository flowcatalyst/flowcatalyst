<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminSubscriptionBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse400
     */
    private $apiAdminSubscriptionsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse400 $apiAdminSubscriptionsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsPostResponse400 = $apiAdminSubscriptionsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsPostResponse400
    {
        return $this->apiAdminSubscriptionsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}