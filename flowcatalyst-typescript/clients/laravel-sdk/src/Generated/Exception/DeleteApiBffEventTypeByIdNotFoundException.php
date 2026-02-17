<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdDeleteResponse404
     */
    private $apiBffEventTypesIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdDeleteResponse404 $apiBffEventTypesIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdDeleteResponse404 = $apiBffEventTypesIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdDeleteResponse404
    {
        return $this->apiBffEventTypesIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}