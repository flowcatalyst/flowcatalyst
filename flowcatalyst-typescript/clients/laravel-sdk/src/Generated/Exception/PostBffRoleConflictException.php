<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffRoleConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesPostResponse409
     */
    private $bffRolesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesPostResponse409 $bffRolesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesPostResponse409 = $bffRolesPostResponse409;
        $this->response = $response;
    }
    public function getBffRolesPostResponse409(): \FlowCatalyst\Generated\Model\BffRolesPostResponse409
    {
        return $this->bffRolesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}