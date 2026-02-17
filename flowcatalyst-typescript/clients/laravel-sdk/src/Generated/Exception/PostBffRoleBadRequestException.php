<?php

namespace FlowCatalyst\Generated\Exception;

class PostBffRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\BffRolesPostResponse400
     */
    private $bffRolesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\BffRolesPostResponse400 $bffRolesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->bffRolesPostResponse400 = $bffRolesPostResponse400;
        $this->response = $response;
    }
    public function getBffRolesPostResponse400(): \FlowCatalyst\Generated\Model\BffRolesPostResponse400
    {
        return $this->bffRolesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}