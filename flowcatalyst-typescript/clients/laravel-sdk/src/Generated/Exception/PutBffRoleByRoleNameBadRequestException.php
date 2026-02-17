<?php

namespace FlowCatalyst\Generated\Exception;

class PutBffRoleByRoleNameBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse400
     */
    private $bffRolesRoleNamePutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse400 $bffRolesRoleNamePutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesRoleNamePutResponse400 = $bffRolesRoleNamePutResponse400;
        $this->response = $response;
    }
    public function getBffRolesRoleNamePutResponse400(): \FlowCatalyst\Generated\Model\BffRolesRoleNamePutResponse400
    {
        return $this->bffRolesRoleNamePutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}