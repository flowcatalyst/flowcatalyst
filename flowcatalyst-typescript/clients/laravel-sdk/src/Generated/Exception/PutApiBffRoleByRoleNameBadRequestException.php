<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiBffRoleByRoleNameBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse400
     */
    private $apiBffRolesRoleNamePutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse400 $apiBffRolesRoleNamePutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesRoleNamePutResponse400 = $apiBffRolesRoleNamePutResponse400;
        $this->response = $response;
    }
    public function getApiBffRolesRoleNamePutResponse400(): \FlowCatalyst\Generated\Model\ApiBffRolesRoleNamePutResponse400
    {
        return $this->apiBffRolesRoleNamePutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}