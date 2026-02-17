<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse404
     */
    private $apiAdminPrincipalsIdRolesGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse404 $apiAdminPrincipalsIdRolesGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesGetResponse404 = $apiAdminPrincipalsIdRolesGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesGetResponse404
    {
        return $this->apiAdminPrincipalsIdRolesGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}