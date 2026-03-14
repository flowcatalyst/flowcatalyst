<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminDispatchPoolsSyncBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse400
     */
    private $apiAdminDispatchPoolsSyncPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse400 $apiAdminDispatchPoolsSyncPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsSyncPostResponse400 = $apiAdminDispatchPoolsSyncPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsSyncPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsSyncPostResponse400
    {
        return $this->apiAdminDispatchPoolsSyncPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}