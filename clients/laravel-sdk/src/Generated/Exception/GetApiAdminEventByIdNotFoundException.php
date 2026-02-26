<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminEventByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse404
     */
    private $apiAdminEventsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse404 $apiAdminEventsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventsIdGetResponse404 = $apiAdminEventsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventsIdGetResponse404
    {
        return $this->apiAdminEventsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}