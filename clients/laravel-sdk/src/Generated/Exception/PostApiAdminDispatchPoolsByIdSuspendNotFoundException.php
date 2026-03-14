<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminDispatchPoolsByIdSuspendNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse404
     */
    private $apiAdminDispatchPoolsIdSuspendPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse404 $apiAdminDispatchPoolsIdSuspendPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdSuspendPostResponse404 = $apiAdminDispatchPoolsIdSuspendPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdSuspendPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdSuspendPostResponse404
    {
        return $this->apiAdminDispatchPoolsIdSuspendPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}