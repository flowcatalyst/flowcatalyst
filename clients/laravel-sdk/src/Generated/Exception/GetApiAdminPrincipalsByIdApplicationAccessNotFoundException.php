<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalsByIdApplicationAccessNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse404
     */
    private $apiAdminPrincipalsIdApplicationAccessGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse404 $apiAdminPrincipalsIdApplicationAccessGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdApplicationAccessGetResponse404 = $apiAdminPrincipalsIdApplicationAccessGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdApplicationAccessGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessGetResponse404
    {
        return $this->apiAdminPrincipalsIdApplicationAccessGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}