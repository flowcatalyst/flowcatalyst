<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminDispatchPoolsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse404
     */
    private $apiAdminDispatchPoolsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse404 $apiAdminDispatchPoolsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdActivatePostResponse404 = $apiAdminDispatchPoolsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdActivatePostResponse404
    {
        return $this->apiAdminDispatchPoolsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}