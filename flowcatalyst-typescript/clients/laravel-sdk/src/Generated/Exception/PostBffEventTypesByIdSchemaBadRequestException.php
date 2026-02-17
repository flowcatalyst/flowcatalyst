<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemaBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse400
     */
    private $bffEventTypesIdSchemasPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse400 $bffEventTypesIdSchemasPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasPostResponse400 = $bffEventTypesIdSchemasPostResponse400;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasPostResponse400(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse400
    {
        return $this->bffEventTypesIdSchemasPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}