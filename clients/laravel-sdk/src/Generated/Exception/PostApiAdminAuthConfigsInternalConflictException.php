<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminAuthConfigsInternalConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409
     */
    private $apiAdminAuthConfigsInternalPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409 $apiAdminAuthConfigsInternalPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminAuthConfigsInternalPostResponse409 = $apiAdminAuthConfigsInternalPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminAuthConfigsInternalPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminAuthConfigsInternalPostResponse409
    {
        return $this->apiAdminAuthConfigsInternalPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}