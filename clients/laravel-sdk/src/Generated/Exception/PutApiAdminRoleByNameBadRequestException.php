<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminRoleByNameBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400
     */
    private $apiAdminRolesNamePutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400 $apiAdminRolesNamePutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesNamePutResponse400 = $apiAdminRolesNamePutResponse400;
        $this->response = $response;
    }
    public function getApiAdminRolesNamePutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse400
    {
        return $this->apiAdminRolesNamePutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}