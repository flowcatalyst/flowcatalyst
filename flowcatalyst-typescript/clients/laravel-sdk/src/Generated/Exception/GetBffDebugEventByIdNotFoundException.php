<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffDebugEventByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse404
     */
    private $bffDebugEventsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse404 $bffDebugEventsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffDebugEventsIdGetResponse404 = $bffDebugEventsIdGetResponse404;
        $this->response = $response;
    }
    public function getBffDebugEventsIdGetResponse404(): \FlowCatalyst\Generated\Model\BffDebugEventsIdGetResponse404
    {
        return $this->bffDebugEventsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}