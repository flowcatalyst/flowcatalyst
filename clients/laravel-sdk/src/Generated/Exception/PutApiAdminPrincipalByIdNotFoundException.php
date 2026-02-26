<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse404
     */
    private $apiAdminPrincipalsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse404 $apiAdminPrincipalsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdPutResponse404 = $apiAdminPrincipalsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse404
    {
        return $this->apiAdminPrincipalsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}