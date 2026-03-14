<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiAdminPrincipalsByIdClientAccessNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404
     */
    private $apiAdminPrincipalsIdClientAccessPostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404 $apiAdminPrincipalsIdClientAccessPostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdClientAccessPostResponse404 = $apiAdminPrincipalsIdClientAccessPostResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdClientAccessPostResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessPostResponse404
    {
        return $this->apiAdminPrincipalsIdClientAccessPostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}