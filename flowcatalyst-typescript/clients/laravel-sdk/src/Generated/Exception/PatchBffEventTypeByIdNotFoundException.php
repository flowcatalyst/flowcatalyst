<?php

namespace FlowCatalyst\Generated\Exception;

class PatchBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse404
     */
    private $bffEventTypesIdPatchResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse404 $bffEventTypesIdPatchResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdPatchResponse404 = $bffEventTypesIdPatchResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdPatchResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse404
    {
        return $this->bffEventTypesIdPatchResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}