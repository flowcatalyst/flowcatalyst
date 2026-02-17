<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffRoleConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse409
     */
    private $apiBffRolesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesPostResponse409 $apiBffRolesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesPostResponse409 = $apiBffRolesPostResponse409;
        $this->response = $response;
    }
    public function getApiBffRolesPostResponse409(): \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse409
    {
        return $this->apiBffRolesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}