<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminDispatchPoolByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse404
     */
    private $apiAdminDispatchPoolsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse404 $apiAdminDispatchPoolsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsIdGetResponse404 = $apiAdminDispatchPoolsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsIdGetResponse404
    {
        return $this->apiAdminDispatchPoolsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}