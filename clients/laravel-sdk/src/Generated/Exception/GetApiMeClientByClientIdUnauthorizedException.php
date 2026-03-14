<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientByClientIdUnauthorizedException extends UnauthorizedException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401
     */
    private $apiMeClientsClientIdGetResponse401;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401 $apiMeClientsClientIdGetResponse401, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdGetResponse401 = $apiMeClientsClientIdGetResponse401;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdGetResponse401(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse401
    {
        return $this->apiMeClientsClientIdGetResponse401;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}