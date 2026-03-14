<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse409
     */
    private $apiAdminApplicationsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse409 $apiAdminApplicationsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsPostResponse409 = $apiAdminApplicationsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminApplicationsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsPostResponse409
    {
        return $this->apiAdminApplicationsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}