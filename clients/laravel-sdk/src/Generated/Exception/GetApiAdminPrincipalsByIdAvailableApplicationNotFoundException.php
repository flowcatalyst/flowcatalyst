<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminPrincipalsByIdAvailableApplicationNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404
     */
    private $apiAdminPrincipalsIdAvailableApplicationsGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404 $apiAdminPrincipalsIdAvailableApplicationsGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdAvailableApplicationsGetResponse404 = $apiAdminPrincipalsIdAvailableApplicationsGetResponse404;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdAvailableApplicationsGetResponse404(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdAvailableApplicationsGetResponse404
    {
        return $this->apiAdminPrincipalsIdAvailableApplicationsGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}