<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalsByIdClientAccessNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse404
     */
    private $apiAdminPrincipalsIdClientAccessGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse404 $apiAdminPrincipalsIdClientAccessGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdClientAccessGetResponse404 = $apiAdminPrincipalsIdClientAccessGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdClientAccessGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdClientAccessGetResponse404
    {
        return $this->apiAdminPrincipalsIdClientAccessGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}