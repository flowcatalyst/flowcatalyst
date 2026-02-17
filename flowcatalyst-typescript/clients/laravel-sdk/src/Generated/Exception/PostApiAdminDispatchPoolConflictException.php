<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminDispatchPoolConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse409
     */
    private $apiAdminDispatchPoolsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse409 $apiAdminDispatchPoolsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminDispatchPoolsPostResponse409 = $apiAdminDispatchPoolsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminDispatchPoolsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminDispatchPoolsPostResponse409
    {
        return $this->apiAdminDispatchPoolsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}