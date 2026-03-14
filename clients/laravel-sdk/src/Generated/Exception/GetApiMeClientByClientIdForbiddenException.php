<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientByClientIdForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403
     */
    private $apiMeClientsClientIdGetResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403 $apiMeClientsClientIdGetResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdGetResponse403 = $apiMeClientsClientIdGetResponse403;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdGetResponse403(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse403
    {
        return $this->apiMeClientsClientIdGetResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}