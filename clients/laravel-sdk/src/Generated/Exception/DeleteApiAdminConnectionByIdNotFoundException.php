<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminConnectionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse404
     */
    private $apiAdminConnectionsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse404 $apiAdminConnectionsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdDeleteResponse404 = $apiAdminConnectionsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdDeleteResponse404
    {
        return $this->apiAdminConnectionsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}