<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalsByIdApplicationAccessBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse400
     */
    private $apiAdminPrincipalsIdApplicationAccessPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse400 $apiAdminPrincipalsIdApplicationAccessPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdApplicationAccessPutResponse400 = $apiAdminPrincipalsIdApplicationAccessPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdApplicationAccessPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdApplicationAccessPutResponse400
    {
        return $this->apiAdminPrincipalsIdApplicationAccessPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}