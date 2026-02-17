<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiSdkPrincipalsByIdClientByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404
     */
    private $apiSdkPrincipalsIdClientsClientIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404 $apiSdkPrincipalsIdClientsClientIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdClientsClientIdDeleteResponse404 = $apiSdkPrincipalsIdClientsClientIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdClientsClientIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdDeleteResponse404
    {
        return $this->apiSdkPrincipalsIdClientsClientIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}