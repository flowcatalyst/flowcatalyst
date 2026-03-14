<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiMeClientByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404
     */
    private $apiMeClientsClientIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404 $apiMeClientsClientIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiMeClientsClientIdGetResponse404 = $apiMeClientsClientIdGetResponse404;
        $this->response = $response;
    }
    public function getApiMeClientsClientIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiMeClientsClientIdGetResponse404
    {
        return $this->apiMeClientsClientIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}