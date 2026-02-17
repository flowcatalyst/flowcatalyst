<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiSdkPrincipalsByIdClientNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse404
     */
    private $apiSdkPrincipalsIdClientsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse404 $apiSdkPrincipalsIdClientsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdClientsGetResponse404 = $apiSdkPrincipalsIdClientsGetResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdClientsGetResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsGetResponse404
    {
        return $this->apiSdkPrincipalsIdClientsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}