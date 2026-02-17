<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminDispatchPoolBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse400
     */
    private $apiAdminDispatchPoolsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse400 $apiAdminDispatchPoolsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsPostResponse400 = $apiAdminDispatchPoolsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse400
    {
        return $this->apiAdminDispatchPoolsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}