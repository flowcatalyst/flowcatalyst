<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminConnectionByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdGetResponse404
     */
    private $apiAdminConnectionsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConnectionsIdGetResponse404 $apiAdminConnectionsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConnectionsIdGetResponse404 = $apiAdminConnectionsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminConnectionsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConnectionsIdGetResponse404
    {
        return $this->apiAdminConnectionsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}