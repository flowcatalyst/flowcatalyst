<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdRoleBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse400
     */
    private $apiAdminPrincipalsIdRolesPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse400 $apiAdminPrincipalsIdRolesPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdRolesPostResponse400 = $apiAdminPrincipalsIdRolesPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdRolesPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdRolesPostResponse400
    {
        return $this->apiAdminPrincipalsIdRolesPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}