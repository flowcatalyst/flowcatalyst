<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkPrincipalsByIdRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400
     */
    private $apiSdkPrincipalsIdRolesPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400 $apiSdkPrincipalsIdRolesPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdRolesPutResponse400 = $apiSdkPrincipalsIdRolesPutResponse400;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdRolesPutResponse400(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesPutResponse400
    {
        return $this->apiSdkPrincipalsIdRolesPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}