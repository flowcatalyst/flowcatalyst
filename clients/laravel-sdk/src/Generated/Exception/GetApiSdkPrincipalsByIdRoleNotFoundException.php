<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiSdkPrincipalsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse404
     */
    private $apiSdkPrincipalsIdRolesGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse404 $apiSdkPrincipalsIdRolesGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdRolesGetResponse404 = $apiSdkPrincipalsIdRolesGetResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdRolesGetResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdRolesGetResponse404
    {
        return $this->apiSdkPrincipalsIdRolesGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}