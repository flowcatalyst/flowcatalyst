<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminClientsByIdActivateBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse400
     */
    private $apiAdminClientsIdActivatePostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse400 $apiAdminClientsIdActivatePostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminClientsIdActivatePostResponse400 = $apiAdminClientsIdActivatePostResponse400;
        $this->response = $response;
    }
    public function getApiAdminClientsIdActivatePostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminClientsIdActivatePostResponse400
    {
        return $this->apiAdminClientsIdActivatePostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}