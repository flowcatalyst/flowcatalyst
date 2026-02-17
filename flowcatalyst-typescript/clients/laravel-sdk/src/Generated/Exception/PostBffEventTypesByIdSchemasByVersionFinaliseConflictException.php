<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemasByVersionFinaliseConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse409
     */
    private $bffEventTypesIdSchemasVersionFinalisePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse409 $bffEventTypesIdSchemasVersionFinalisePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasVersionFinalisePostResponse409 = $bffEventTypesIdSchemasVersionFinalisePostResponse409;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasVersionFinalisePostResponse409(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionFinalisePostResponse409
    {
        return $this->bffEventTypesIdSchemasVersionFinalisePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}