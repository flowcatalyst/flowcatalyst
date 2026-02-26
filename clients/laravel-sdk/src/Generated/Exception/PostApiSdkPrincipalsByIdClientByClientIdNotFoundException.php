<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsByIdClientByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404
     */
    private $apiSdkPrincipalsIdClientsClientIdPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404 $apiSdkPrincipalsIdClientsClientIdPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdClientsClientIdPostResponse404 = $apiSdkPrincipalsIdClientsClientIdPostResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdClientsClientIdPostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse404
    {
        return $this->apiSdkPrincipalsIdClientsClientIdPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}