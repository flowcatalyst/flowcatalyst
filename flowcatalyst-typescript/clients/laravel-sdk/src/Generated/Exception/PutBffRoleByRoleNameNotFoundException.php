<?php

namespace FlowCatalyst\Generated\Exception;

class PutBffRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse404
     */
    private $bffRolesRoleNamePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse404 $bffRolesRoleNamePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesRoleNamePutResponse404 = $bffRolesRoleNamePutResponse404;
        $this->response = $response;
    }
    public function getBffRolesRoleNamePutResponse404(): \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse404
    {
        return $this->bffRolesRoleNamePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}