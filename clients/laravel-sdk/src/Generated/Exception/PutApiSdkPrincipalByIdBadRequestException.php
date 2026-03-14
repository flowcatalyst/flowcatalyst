<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiSdkPrincipalByIdBadRequestException extends BadRequestException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse400
     */
    private $apiSdkPrincipalsIdPutResponse400;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse400 $apiSdkPrincipalsIdPutResponse400, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdPutResponse400 = $apiSdkPrincipalsIdPutResponse400;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdPutResponse400(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdPutResponse400
    {
        return $this->apiSdkPrincipalsIdPutResponse400;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}