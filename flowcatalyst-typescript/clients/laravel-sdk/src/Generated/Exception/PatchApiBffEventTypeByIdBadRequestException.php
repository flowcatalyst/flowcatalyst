<?php

namespace FlowCatalyst\Generated\Exception;

class PatchApiBffEventTypeByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse400
     */
    private $apiBffEventTypesIdPatchResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse400 $apiBffEventTypesIdPatchResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdPatchResponse400 = $apiBffEventTypesIdPatchResponse400;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdPatchResponse400(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse400
    {
        return $this->apiBffEventTypesIdPatchResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}