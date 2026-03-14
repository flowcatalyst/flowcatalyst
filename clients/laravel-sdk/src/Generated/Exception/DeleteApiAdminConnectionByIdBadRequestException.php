<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminConnectionByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse400
     */
    private $apiAdminConnectionsIdDeleteResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse400 $apiAdminConnectionsIdDeleteResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdDeleteResponse400 = $apiAdminConnectionsIdDeleteResponse400;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdDeleteResponse400(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse400
    {
        return $this->apiAdminConnectionsIdDeleteResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}