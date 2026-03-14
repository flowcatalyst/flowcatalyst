<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkRoleConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse409
     */
    private $apiSdkRolesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse409 $apiSdkRolesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesPostResponse409 = $apiSdkRolesPostResponse409;
        $this->response = $response;
    }
    public function getApiSdkRolesPostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkRolesPostResponse409
    {
        return $this->apiSdkRolesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}