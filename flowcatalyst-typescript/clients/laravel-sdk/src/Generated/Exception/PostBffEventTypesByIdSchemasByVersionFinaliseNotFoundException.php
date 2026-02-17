<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemasByVersionFinaliseNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse404
     */
    private $bffEventTypesIdSchemasVersionFinalisePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse404 $bffEventTypesIdSchemasVersionFinalisePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasVersionFinalisePostResponse404 = $bffEventTypesIdSchemasVersionFinalisePostResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasVersionFinalisePostResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse404
    {
        return $this->bffEventTypesIdSchemasVersionFinalisePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}