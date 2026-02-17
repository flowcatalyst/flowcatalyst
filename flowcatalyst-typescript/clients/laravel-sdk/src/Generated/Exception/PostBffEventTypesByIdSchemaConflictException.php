<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemaConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse409
     */
    private $bffEventTypesIdSchemasPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse409 $bffEventTypesIdSchemasPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasPostResponse409 = $bffEventTypesIdSchemasPostResponse409;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasPostResponse409(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse409
    {
        return $this->bffEventTypesIdSchemasPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}