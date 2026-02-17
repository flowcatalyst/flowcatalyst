<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemasByVersionFinaliseNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404
     */
    private $apiBffEventTypesIdSchemasVersionFinalisePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404 $apiBffEventTypesIdSchemasVersionFinalisePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasVersionFinalisePostResponse404 = $apiBffEventTypesIdSchemasVersionFinalisePostResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasVersionFinalisePostResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse404
    {
        return $this->apiBffEventTypesIdSchemasVersionFinalisePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}