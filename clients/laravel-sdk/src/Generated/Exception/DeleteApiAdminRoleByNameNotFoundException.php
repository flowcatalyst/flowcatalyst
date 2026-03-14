<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminRoleByNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesNameDeleteResponse404
     */
    private $apiAdminRolesNameDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesNameDeleteResponse404 $apiAdminRolesNameDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesNameDeleteResponse404 = $apiAdminRolesNameDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminRolesNameDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminRolesNameDeleteResponse404
    {
        return $this->apiAdminRolesNameDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}