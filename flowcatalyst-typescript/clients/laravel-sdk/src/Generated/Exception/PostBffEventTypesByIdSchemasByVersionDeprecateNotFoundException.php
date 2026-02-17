<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffEventTypesByIdSchemasByVersionDeprecateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse404
     */
    private $bffEventTypesIdSchemasVersionDeprecatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse404 $bffEventTypesIdSchemasVersionDeprecatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffEventTypesIdSchemasVersionDeprecatePostResponse404 = $bffEventTypesIdSchemasVersionDeprecatePostResponse404;
        $this->response = $response;
    }
    public function getBffEventTypesIdSchemasVersionDeprecatePostResponse404(): \FlowCatalyst\Generated\Model\BffEventTypesIdSchemasVersionDeprecatePostResponse404
    {
        return $this->bffEventTypesIdSchemasVersionDeprecatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}