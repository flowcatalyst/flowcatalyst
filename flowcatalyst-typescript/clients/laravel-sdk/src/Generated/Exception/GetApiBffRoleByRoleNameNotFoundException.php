<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse404
     */
    private $apiBffRolesRoleNameGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse404 $apiBffRolesRoleNameGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesRoleNameGetResponse404 = $apiBffRolesRoleNameGetResponse404;
        $this->response = $response;
    }
    public function getApiBffRolesRoleNameGetResponse404(): \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameGetResponse404
    {
        return $this->apiBffRolesRoleNameGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}