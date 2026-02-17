<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemaNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404
     */
    private $apiBffEventTypesIdSchemasPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404 $apiBffEventTypesIdSchemasPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasPostResponse404 = $apiBffEventTypesIdSchemasPostResponse404;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasPostResponse404(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse404
    {
        return $this->apiBffEventTypesIdSchemasPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}