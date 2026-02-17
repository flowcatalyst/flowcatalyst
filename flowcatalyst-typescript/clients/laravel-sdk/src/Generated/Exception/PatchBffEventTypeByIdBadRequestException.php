<?php

namespace FlowCatalyst\Generated\Exception;

class PatchBffEventTypeByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse400
     */
    private $bffEventTypesIdPatchResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse400 $bffEventTypesIdPatchResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdPatchResponse400 = $bffEventTypesIdPatchResponse400;
        $this->response = $response;
    }
    public function getBffEventTypesIdPatchResponse400(): \FlowCatalyst\Generated\Model\BffEventTypesIdPatchResponse400
    {
        return $this->bffEventTypesIdPatchResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}