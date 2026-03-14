<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminConnectionBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse400
     */
    private $apiAdminConnectionsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse400 $apiAdminConnectionsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsPostResponse400 = $apiAdminConnectionsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminConnectionsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsPostResponse400
    {
        return $this->apiAdminConnectionsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}