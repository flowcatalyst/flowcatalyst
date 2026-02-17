<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemaNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse404
     */
    private $bffEventTypesIdSchemasPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse404 $bffEventTypesIdSchemasPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasPostResponse404 = $bffEventTypesIdSchemasPostResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasPostResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasPostResponse404
    {
        return $this->bffEventTypesIdSchemasPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}