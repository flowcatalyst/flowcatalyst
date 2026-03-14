<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientsByClientIdApplicationNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404
     */
    private $apiMeClientsClientIdApplicationsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404 $apiMeClientsClientIdApplicationsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdApplicationsGetResponse404 = $apiMeClientsClientIdApplicationsGetResponse404;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdApplicationsGetResponse404(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse404
    {
        return $this->apiMeClientsClientIdApplicationsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}