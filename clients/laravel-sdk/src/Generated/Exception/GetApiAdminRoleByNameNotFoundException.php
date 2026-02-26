<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminRoleByNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse404
     */
    private $apiAdminRolesNameGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse404 $apiAdminRolesNameGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesNameGetResponse404 = $apiAdminRolesNameGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminRolesNameGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminRolesNameGetResponse404
    {
        return $this->apiAdminRolesNameGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}