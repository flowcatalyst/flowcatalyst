<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameDeleteResponse404
     */
    private $apiBffRolesRoleNameDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesRoleNameDeleteResponse404 $apiBffRolesRoleNameDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesRoleNameDeleteResponse404 = $apiBffRolesRoleNameDeleteResponse404;
        $this->response = $response;
    }
    public function getApiBffRolesRoleNameDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiBffRolesRoleNameDeleteResponse404
    {
        return $this->apiBffRolesRoleNameDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}