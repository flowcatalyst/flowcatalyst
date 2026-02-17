<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffEventByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventsIdGetResponse404
     */
    private $bffEventsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventsIdGetResponse404 $bffEventsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventsIdGetResponse404 = $bffEventsIdGetResponse404;
        $this->response = $response;
    }
    public function getBffEventsIdGetResponse404(): \FlowCatalyst\Generated\Model\BffEventsIdGetResponse404
    {
        return $this->bffEventsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}