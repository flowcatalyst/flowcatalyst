<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminDispatchPoolByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdDeleteResponse404
     */
    private $apiAdminDispatchPoolsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdDeleteResponse404 $apiAdminDispatchPoolsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdDeleteResponse404 = $apiAdminDispatchPoolsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdDeleteResponse404
    {
        return $this->apiAdminDispatchPoolsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}