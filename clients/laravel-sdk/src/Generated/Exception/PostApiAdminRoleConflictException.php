<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminRoleConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse409
     */
    private $apiAdminRolesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse409 $apiAdminRolesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesPostResponse409 = $apiAdminRolesPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminRolesPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse409
    {
        return $this->apiAdminRolesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}