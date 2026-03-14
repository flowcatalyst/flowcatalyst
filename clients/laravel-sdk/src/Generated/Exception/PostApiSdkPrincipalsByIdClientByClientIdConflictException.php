<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsByIdClientByClientIdConflictException extends ConflictException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409
     */
    private $apiSdkPrincipalsIdClientsClientIdPostResponse409;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409 $apiSdkPrincipalsIdClientsClientIdPostResponse409, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdClientsClientIdPostResponse409 = $apiSdkPrincipalsIdClientsClientIdPostResponse409;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdClientsClientIdPostResponse409(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdClientsClientIdPostResponse409
    {
        return $this->apiSdkPrincipalsIdClientsClientIdPostResponse409;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}