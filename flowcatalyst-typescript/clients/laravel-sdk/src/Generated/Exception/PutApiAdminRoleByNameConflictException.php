<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminRoleByNameConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409
     */
    private $apiAdminRolesNamePutResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409 $apiAdminRolesNamePutResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesNamePutResponse409 = $apiAdminRolesNamePutResponse409;
        $this->response = $response;
    }
    public function getApiAdminRolesNamePutResponse409(): \FlowCatalyst\Generated\Model\ApiAdminRolesNamePutResponse409
    {
        return $this->apiAdminRolesNamePutResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}