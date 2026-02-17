<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse404
     */
    private $apiAdminPrincipalsIdRolesPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse404 $apiAdminPrincipalsIdRolesPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesPostResponse404 = $apiAdminPrincipalsIdRolesPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse404
    {
        return $this->apiAdminPrincipalsIdRolesPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}