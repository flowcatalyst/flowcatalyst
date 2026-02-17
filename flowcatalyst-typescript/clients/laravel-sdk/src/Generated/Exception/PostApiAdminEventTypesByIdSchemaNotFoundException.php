<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdSchemaNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse404
     */
    private $apiAdminEventTypesIdSchemasPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse404 $apiAdminEventTypesIdSchemasPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdSchemasPostResponse404 = $apiAdminEventTypesIdSchemasPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdSchemasPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse404
    {
        return $this->apiAdminEventTypesIdSchemasPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}