<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminApplicationsByIdDeactivateConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse409
     */
    private $apiAdminApplicationsIdDeactivatePostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse409 $apiAdminApplicationsIdDeactivatePostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminApplicationsIdDeactivatePostResponse409 = $apiAdminApplicationsIdDeactivatePostResponse409;
        $this->response = $response;
    }
    public function getApiAdminApplicationsIdDeactivatePostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminApplicationsIdDeactivatePostResponse409
    {
        return $this->apiAdminApplicationsIdDeactivatePostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}