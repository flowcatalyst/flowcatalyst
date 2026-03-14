<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdResetPasswordNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse404
     */
    private $apiAdminPrincipalsIdResetPasswordPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse404 $apiAdminPrincipalsIdResetPasswordPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdResetPasswordPostResponse404 = $apiAdminPrincipalsIdResetPasswordPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdResetPasswordPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse404
    {
        return $this->apiAdminPrincipalsIdResetPasswordPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}