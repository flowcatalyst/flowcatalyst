<?php

namespace FlowCatalyst\Generated\Exception;

class PatchApiBffEventTypeByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse404
     */
    private $apiBffEventTypesIdPatchResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse404 $apiBffEventTypesIdPatchResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdPatchResponse404 = $apiBffEventTypesIdPatchResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdPatchResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdPatchResponse404
    {
        return $this->apiBffEventTypesIdPatchResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}