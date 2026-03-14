<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse404
     */
    private $apiAdminPrincipalsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse404 $apiAdminPrincipalsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdGetResponse404 = $apiAdminPrincipalsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdGetResponse404
    {
        return $this->apiAdminPrincipalsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}