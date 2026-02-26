<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientsByClientIdApplicationUnauthorizedException extends UnauthorizedException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401
     */
    private $apiMeClientsClientIdApplicationsGetResponse401;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401 $apiMeClientsClientIdApplicationsGetResponse401, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdApplicationsGetResponse401 = $apiMeClientsClientIdApplicationsGetResponse401;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdApplicationsGetResponse401(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse401
    {
        return $this->apiMeClientsClientIdApplicationsGetResponse401;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}