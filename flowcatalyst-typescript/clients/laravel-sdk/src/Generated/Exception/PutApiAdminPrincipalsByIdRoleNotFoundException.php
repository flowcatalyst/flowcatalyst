<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalsByIdRoleNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse404
     */
    private $apiAdminPrincipalsIdRolesPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse404 $apiAdminPrincipalsIdRolesPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesPutResponse404 = $apiAdminPrincipalsIdRolesPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse404
    {
        return $this->apiAdminPrincipalsIdRolesPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}