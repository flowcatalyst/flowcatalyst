<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminEventTypeConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse409
     */
    private $apiAdminEventTypesPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse409 $apiAdminEventTypesPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminEventTypesPostResponse409 = $apiAdminEventTypesPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminEventTypesPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminEventTypesPostResponse409
    {
        return $this->apiAdminEventTypesPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}