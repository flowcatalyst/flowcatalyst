<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminRoleByNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404
     */
    private $apiAdminRolesNamePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404 $apiAdminRolesNamePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesNamePutResponse404 = $apiAdminRolesNamePutResponse404;
        $this->response = $response;
    }
    public function getApiAdminRolesNamePutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse404
    {
        return $this->apiAdminRolesNamePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}