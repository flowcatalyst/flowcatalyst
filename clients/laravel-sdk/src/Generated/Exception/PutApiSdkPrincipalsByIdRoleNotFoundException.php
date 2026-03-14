<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkPrincipalsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404
     */
    private $apiSdkPrincipalsIdRolesPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404 $apiSdkPrincipalsIdRolesPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdRolesPutResponse404 = $apiSdkPrincipalsIdRolesPutResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdRolesPutResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse404
    {
        return $this->apiSdkPrincipalsIdRolesPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}