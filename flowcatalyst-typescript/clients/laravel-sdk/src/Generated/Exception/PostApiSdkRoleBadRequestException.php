<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse400
     */
    private $apiSdkRolesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse400 $apiSdkRolesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesPostResponse400 = $apiSdkRolesPostResponse400;
        $this->response = $response;
    }
    public function getApiSdkRolesPostResponse400(): \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse400
    {
        return $this->apiSdkRolesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}