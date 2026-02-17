<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffDebugEventByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse404
     */
    private $apiBffDebugEventsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse404 $apiBffDebugEventsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffDebugEventsIdGetResponse404 = $apiBffDebugEventsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiBffDebugEventsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffDebugEventsIdGetResponse404
    {
        return $this->apiBffDebugEventsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}