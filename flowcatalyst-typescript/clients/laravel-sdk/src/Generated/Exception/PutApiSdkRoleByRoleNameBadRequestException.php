<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkRoleByRoleNameBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400
     */
    private $apiSdkRolesRoleNamePutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400 $apiSdkRolesRoleNamePutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkRolesRoleNamePutResponse400 = $apiSdkRolesRoleNamePutResponse400;
        $this->response = $response;
    }
    public function getApiSdkRolesRoleNamePutResponse400(): \FlowCatalyst\Generated\Model\ApiSdkRolesRoleNamePutResponse400
    {
        return $this->apiSdkRolesRoleNamePutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}