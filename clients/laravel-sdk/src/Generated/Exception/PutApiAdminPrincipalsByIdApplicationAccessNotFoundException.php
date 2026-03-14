<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalsByIdApplicationAccessNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse404
     */
    private $apiAdminPrincipalsIdApplicationAccessPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse404 $apiAdminPrincipalsIdApplicationAccessPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdApplicationAccessPutResponse404 = $apiAdminPrincipalsIdApplicationAccessPutResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdApplicationAccessPutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse404
    {
        return $this->apiAdminPrincipalsIdApplicationAccessPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}