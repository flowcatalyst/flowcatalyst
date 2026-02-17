<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffEventTypesByIdSchemaBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400
     */
    private $apiBffEventTypesIdSchemasPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400 $apiBffEventTypesIdSchemasPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffEventTypesIdSchemasPostResponse400 = $apiBffEventTypesIdSchemasPostResponse400;
        $this->response = $response;
    }
    public function getApiBffEventTypesIdSchemasPostResponse400(): \FlowCatalyst\Generated\Model\ApiBffEventTypesIdSchemasPostResponse400
    {
        return $this->apiBffEventTypesIdSchemasPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}