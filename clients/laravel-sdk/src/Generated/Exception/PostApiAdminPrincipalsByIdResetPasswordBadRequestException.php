<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdResetPasswordBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse400
     */
    private $apiAdminPrincipalsIdResetPasswordPostResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse400 $apiAdminPrincipalsIdResetPasswordPostResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdResetPasswordPostResponse400 = $apiAdminPrincipalsIdResetPasswordPostResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdResetPasswordPostResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdResetPasswordPostResponse400
    {
        return $this->apiAdminPrincipalsIdResetPasswordPostResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}