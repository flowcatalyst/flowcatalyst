<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdSchemasByVersionFinaliseNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404
     */
    private $apiAdminEventTypesIdSchemasVersionFinalisePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404 $apiAdminEventTypesIdSchemasVersionFinalisePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdSchemasVersionFinalisePostResponse404 = $apiAdminEventTypesIdSchemasVersionFinalisePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdSchemasVersionFinalisePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionFinalisePostResponse404
    {
        return $this->apiAdminEventTypesIdSchemasVersionFinalisePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}