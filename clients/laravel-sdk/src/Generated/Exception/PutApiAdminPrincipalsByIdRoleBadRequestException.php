<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalsByIdRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse400
     */
    private $apiAdminPrincipalsIdRolesPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse400 $apiAdminPrincipalsIdRolesPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesPutResponse400 = $apiAdminPrincipalsIdRolesPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPutResponse400
    {
        return $this->apiAdminPrincipalsIdRolesPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}