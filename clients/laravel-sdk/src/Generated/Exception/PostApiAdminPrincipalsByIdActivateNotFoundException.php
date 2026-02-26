<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse404
     */
    private $apiAdminPrincipalsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse404 $apiAdminPrincipalsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdActivatePostResponse404 = $apiAdminPrincipalsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdActivatePostResponse404
    {
        return $this->apiAdminPrincipalsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}