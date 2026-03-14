<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse400
     */
    private $apiAdminRolesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse400 $apiAdminRolesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminRolesPostResponse400 = $apiAdminRolesPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminRolesPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminRolesPostResponse400
    {
        return $this->apiAdminRolesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}