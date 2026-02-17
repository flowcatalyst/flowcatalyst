<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiSdkPrincipalByIdNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse404
     */
    private $apiSdkPrincipalsIdGetResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse404 $apiSdkPrincipalsIdGetResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdGetResponse404 = $apiSdkPrincipalsIdGetResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdGetResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdGetResponse404
    {
        return $this->apiSdkPrincipalsIdGetResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}