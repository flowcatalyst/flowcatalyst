<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminSubscriptionByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse400
     */
    private $apiAdminSubscriptionsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse400 $apiAdminSubscriptionsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminSubscriptionsIdPutResponse400 = $apiAdminSubscriptionsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminSubscriptionsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminSubscriptionsIdPutResponse400
    {
        return $this->apiAdminSubscriptionsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}