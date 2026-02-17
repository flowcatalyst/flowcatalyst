<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemasByVersionDeprecateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404
     */
    private $apiBffEventTypesIdSchemasVersionDeprecatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404 $apiBffEventTypesIdSchemasVersionDeprecatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasVersionDeprecatePostResponse404 = $apiBffEventTypesIdSchemasVersionDeprecatePostResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasVersionDeprecatePostResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasVersionDeprecatePostResponse404
    {
        return $this->apiBffEventTypesIdSchemasVersionDeprecatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}