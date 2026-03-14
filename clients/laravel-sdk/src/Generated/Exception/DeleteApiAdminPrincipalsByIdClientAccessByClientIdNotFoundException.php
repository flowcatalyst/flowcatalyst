<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminPrincipalsByIdClientAccessByClientIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404
     */
    private $apiAdminPrincipalsIdClientAccessClientIdDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404 $apiAdminPrincipalsIdClientAccessClientIdDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdClientAccessClientIdDeleteResponse404 = $apiAdminPrincipalsIdClientAccessClientIdDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessClientIdDeleteResponse404
    {
        return $this->apiAdminPrincipalsIdClientAccessClientIdDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}