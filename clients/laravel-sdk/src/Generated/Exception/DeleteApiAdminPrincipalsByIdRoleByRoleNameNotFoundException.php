<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminPrincipalsByIdRoleByRoleNameNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404
     */
    private $apiAdminPrincipalsIdRolesRoleNameDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404 $apiAdminPrincipalsIdRolesRoleNameDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesRoleNameDeleteResponse404 = $apiAdminPrincipalsIdRolesRoleNameDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesRoleNameDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesRoleNameDeleteResponse404
    {
        return $this->apiAdminPrincipalsIdRolesRoleNameDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}