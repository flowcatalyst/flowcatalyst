<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse404
     */
    private $apiBffEventTypesIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse404 $apiBffEventTypesIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdGetResponse404 = $apiBffEventTypesIdGetResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdGetResponse404
    {
        return $this->apiBffEventTypesIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}