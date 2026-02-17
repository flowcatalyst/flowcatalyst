<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemasByVersionDeprecateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409
     */
    private $apiBffEventTypesIdSchemasVersionDeprecatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409 $apiBffEventTypesIdSchemasVersionDeprecatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasVersionDeprecatePostResponse409 = $apiBffEventTypesIdSchemasVersionDeprecatePostResponse409;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasVersionDeprecatePostResponse409(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse409
    {
        return $this->apiBffEventTypesIdSchemasVersionDeprecatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}