<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminPrincipalByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse400
     */
    private $apiAdminPrincipalsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse400 $apiAdminPrincipalsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminPrincipalsIdPutResponse400 = $apiAdminPrincipalsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiAdminPrincipalsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiAdminPrincipalsIdPutResponse400
    {
        return $this->apiAdminPrincipalsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}