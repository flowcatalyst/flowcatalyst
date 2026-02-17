<?php

namespace FlowCatalyst\Generated\Exception;

class GetBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse404
     */
    private $bffEventTypesIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse404 $bffEventTypesIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdGetResponse404 = $bffEventTypesIdGetResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdGetResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdGetResponse404
    {
        return $this->bffEventTypesIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}