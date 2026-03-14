<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdDeactivateBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse400
     */
    private $apiAdminClientsIdDeactivatePostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse400 $apiAdminClientsIdDeactivatePostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdDeactivatePostResponse400 = $apiAdminClientsIdDeactivatePostResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsIdDeactivatePostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdDeactivatePostResponse400
    {
        return $this->apiAdminClientsIdDeactivatePostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}