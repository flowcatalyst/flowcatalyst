<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypesByIdCodegenBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse400
     */
    private $apiAdminEventTypesIdCodegenPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse400 $apiAdminEventTypesIdCodegenPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesIdCodegenPostResponse400 = $apiAdminEventTypesIdCodegenPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminEventTypesIdCodegenPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesIdCodegenPostResponse400
    {
        return $this->apiAdminEventTypesIdCodegenPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}