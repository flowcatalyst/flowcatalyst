<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdSchemaBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse400
     */
    private $apiAdminEventTypesIdSchemasPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse400 $apiAdminEventTypesIdSchemasPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdSchemasPostResponse400 = $apiAdminEventTypesIdSchemasPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdSchemasPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasPostResponse400
    {
        return $this->apiAdminEventTypesIdSchemasPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}