<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemasByVersionDeprecateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse409
     */
    private $bffEventTypesIdSchemasVersionDeprecatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse409 $bffEventTypesIdSchemasVersionDeprecatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasVersionDeprecatePostResponse409 = $bffEventTypesIdSchemasVersionDeprecatePostResponse409;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasVersionDeprecatePostResponse409(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse409
    {
        return $this->bffEventTypesIdSchemasVersionDeprecatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}