<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdCodegenNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse404
     */
    private $apiAdminEventTypesIdCodegenPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse404 $apiAdminEventTypesIdCodegenPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdCodegenPostResponse404 = $apiAdminEventTypesIdCodegenPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdCodegenPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse404
    {
        return $this->apiAdminEventTypesIdCodegenPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}