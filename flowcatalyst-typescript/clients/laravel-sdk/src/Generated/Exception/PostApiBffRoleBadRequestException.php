<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiBffRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse400
     */
    private $apiBffRolesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiBffRolesPostResponse400 $apiBffRolesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiBffRolesPostResponse400 = $apiBffRolesPostResponse400;
        $this->response = $response;
    }
    public function getApiBffRolesPostResponse400(): \FlowCatalyst\Generated\Model\ApiBffRolesPostResponse400
    {
        return $this->apiBffRolesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}