<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminDispatchPoolByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse404
     */
    private $apiAdminDispatchPoolsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse404 $apiAdminDispatchPoolsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdPutResponse404 = $apiAdminDispatchPoolsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdPutResponse404
    {
        return $this->apiAdminDispatchPoolsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}