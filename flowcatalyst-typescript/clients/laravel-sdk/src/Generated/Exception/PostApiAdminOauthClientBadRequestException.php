<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminOauthClientBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse400
     */
    private $apiAdminOauthClientsPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse400 $apiAdminOauthClientsPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminOauthClientsPostResponse400 = $apiAdminOauthClientsPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminOauthClientsPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminOauthClientsPostResponse400
    {
        return $this->apiAdminOauthClientsPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}