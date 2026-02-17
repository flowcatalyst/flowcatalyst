<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdDeleteResponse404
     */
    private $bffEventTypesIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdDeleteResponse404 $bffEventTypesIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdDeleteResponse404 = $bffEventTypesIdDeleteResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdDeleteResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdDeleteResponse404
    {
        return $this->bffEventTypesIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}