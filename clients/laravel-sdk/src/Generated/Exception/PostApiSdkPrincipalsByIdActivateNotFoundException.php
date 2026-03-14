<?php

namespace FlowCatalyst\Generated\Exception;

class PostApiSdkPrincipalsByIdActivateNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse404
     */
    private $apiSdkPrincipalsIdActivatePostResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse404 $apiSdkPrincipalsIdActivatePostResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiSdkPrincipalsIdActivatePostResponse404 = $apiSdkPrincipalsIdActivatePostResponse404;
        $this->response = $response;
    }
    public function getApiSdkPrincipalsIdActivatePostResponse404(): \FlowCatalyst\Generated\Model\ApiSdkPrincipalsIdActivatePostResponse404
    {
        return $this->apiSdkPrincipalsIdActivatePostResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}