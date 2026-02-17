<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientsByClientIdApplicationForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403
     */
    private $apiMeClientsClientIdApplicationsGetResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403 $apiMeClientsClientIdApplicationsGetResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdApplicationsGetResponse403 = $apiMeClientsClientIdApplicationsGetResponse403;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdApplicationsGetResponse403(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdApplicationsGetResponse403
    {
        return $this->apiMeClientsClientIdApplicationsGetResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}