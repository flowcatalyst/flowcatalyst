<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdDeactivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse404
     */
    private $apiAdminPrincipalsIdDeactivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse404 $apiAdminPrincipalsIdDeactivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdDeactivatePostResponse404 = $apiAdminPrincipalsIdDeactivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdDeactivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeactivatePostResponse404
    {
        return $this->apiAdminPrincipalsIdDeactivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}