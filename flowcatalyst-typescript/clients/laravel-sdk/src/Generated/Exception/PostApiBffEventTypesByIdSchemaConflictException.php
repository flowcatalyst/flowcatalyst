<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemaConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409
     */
    private $apiBffEventTypesIdSchemasPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409 $apiBffEventTypesIdSchemasPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasPostResponse409 = $apiBffEventTypesIdSchemasPostResponse409;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasPostResponse409(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse409
    {
        return $this->apiBffEventTypesIdSchemasPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}