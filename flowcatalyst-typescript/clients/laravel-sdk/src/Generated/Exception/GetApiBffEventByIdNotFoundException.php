<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffEventByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse404
     */
    private $apiBffEventsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse404 $apiBffEventsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventsIdGetResponse404 = $apiBffEventsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiBffEventsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventsIdGetResponse404
    {
        return $this->apiBffEventsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}