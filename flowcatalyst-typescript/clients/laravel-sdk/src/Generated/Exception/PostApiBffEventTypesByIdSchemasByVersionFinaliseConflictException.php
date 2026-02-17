<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemasByVersionFinaliseConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409
     */
    private $apiBffEventTypesIdSchemasVersionFinalisePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409 $apiBffEventTypesIdSchemasVersionFinalisePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasVersionFinalisePostResponse409 = $apiBffEventTypesIdSchemasVersionFinalisePostResponse409;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasVersionFinalisePostResponse409(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionFinalisePostResponse409
    {
        return $this->apiBffEventTypesIdSchemasVersionFinalisePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}