<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdSchemasByVersionDeprecateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404
     */
    private $apiAdminEventTypesIdSchemasVersionDeprecatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404 $apiAdminEventTypesIdSchemasVersionDeprecatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdSchemasVersionDeprecatePostResponse404 = $apiAdminEventTypesIdSchemasVersionDeprecatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdSchemasVersionDeprecatePostResponse404
    {
        return $this->apiAdminEventTypesIdSchemasVersionDeprecatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}