<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse404
     */
    private $apiBffRolesRoleNamePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse404 $apiBffRolesRoleNamePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesRoleNamePutResponse404 = $apiBffRolesRoleNamePutResponse404;
        $this->response = $response;
    }
    public function getApiBffRolesRoleNamePutResponse404(): \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse404
    {
        return $this->apiBffRolesRoleNamePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}