<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminPrincipalByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeleteResponse404
     */
    private $apiAdminPrincipalsIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeleteResponse404 $apiAdminPrincipalsIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdDeleteResponse404 = $apiAdminPrincipalsIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdDeleteResponse404
    {
        return $this->apiAdminPrincipalsIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}