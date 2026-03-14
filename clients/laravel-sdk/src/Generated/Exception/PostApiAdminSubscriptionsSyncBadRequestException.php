<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminSubscriptionsSyncBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse400
     */
    private $apiAdminSubscriptionsSyncPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse400 $apiAdminSubscriptionsSyncPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsSyncPostResponse400 = $apiAdminSubscriptionsSyncPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsSyncPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsSyncPostResponse400
    {
        return $this->apiAdminSubscriptionsSyncPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}