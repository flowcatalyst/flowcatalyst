<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse409
     */
    private $apiAdminOauthClientsPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse409 $apiAdminOauthClientsPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsPostResponse409 = $apiAdminOauthClientsPostResponse409;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsPostResponse409(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse409
    {
        return $this->apiAdminOauthClientsPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}