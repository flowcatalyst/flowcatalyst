<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkPrincipalByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse404
     */
    private $apiSdkPrincipalsIdPutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse404 $apiSdkPrincipalsIdPutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdPutResponse404 = $apiSdkPrincipalsIdPutResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdPutResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse404
    {
        return $this->apiSdkPrincipalsIdPutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}