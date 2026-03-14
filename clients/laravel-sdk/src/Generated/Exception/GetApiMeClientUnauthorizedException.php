<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientUnauthorizedException extends UnauthorizedException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse401
     */
    private $apiMeClientsGetResponse401;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsGetResponse401 $apiMeClientsGetResponse401, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsGetResponse401 = $apiMeClientsGetResponse401;
        $this->response = $response;
    }
    public function getApiMeClientsGetResponse401(): \FlowCatalyst\Generated\Model\ApiMeClientsGetResponse401
    {
        return $this->apiMeClientsGetResponse401;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}